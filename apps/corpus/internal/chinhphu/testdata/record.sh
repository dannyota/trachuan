#!/bin/sh
# Records vanban.chinhphu.vn fixtures at 1 request per second. Cookies and
# postback tokens stay in memory; saved pages carry placeholders instead of
# __VIEWSTATE and __EVENTVALIDATION values. It records what the site returns
# and stops only on transport failure.
set -eu

cd "$(dirname "$0")"
python3 -I - <<'PY'
import http.cookiejar
import json
import re
import time
import urllib.error
import urllib.parse
import urllib.request

UA = "trachuan/0.0 (+https://github.com/dannyota/trachuan)"
SITE = "https://vanban.chinhphu.vn/"
TOKENS = ("__VIEWSTATE", "__EVENTVALIDATION", "__VIEWSTATEGENERATOR")
last = [0.0]


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


opener = urllib.request.build_opener(
    urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), NoRedirect
)


def request(url, form=None, method=None, headers=None):
    wait = 1.0 - (time.monotonic() - last[0])
    if wait > 0:
        time.sleep(wait)
    last[0] = time.monotonic()
    head = {"User-Agent": UA}
    head.update(headers or {})
    body = None
    if form is not None:
        body = urllib.parse.urlencode(form).encode()
        head["Content-Type"] = "application/x-www-form-urlencoded"
        head["Referer"] = SITE
    req = urllib.request.Request(url, data=body, headers=head, method=method)
    try:
        with opener.open(req, timeout=120) as resp:
            return resp.status, resp.headers, resp.read()
    except urllib.error.HTTPError as err:
        return err.code, err.headers, err.read()


def fields(page):
    out = {}
    for tag in re.findall(r"<input\b[^>]*>", page):
        name = re.search(r'name="([^"]+)"', tag)
        kind = re.search(r'type="([^"]+)"', tag)
        if not name or (kind and kind.group(1) in ("submit", "image", "button", "checkbox", "radio")):
            continue
        value = re.search(r'value="([^"]*)"', tag)
        out[unescape(name.group(1))] = unescape(value.group(1)) if value else ""
    for name, body in re.findall(r'<select name="([^"]+)"[^>]*>(.*?)</select>', page, re.S):
        chosen = re.search(r'<option selected="selected" value="([^"]*)"', body)
        out[unescape(name)] = chosen.group(1) if chosen else "0"
    return out


def unescape(text):
    return text.replace("&amp;", "&").replace("&#39;", "'").replace("&quot;", '"')


def token(page, name):
    found = re.search(r'id="%s" value="([^"]*)"' % name, page)
    return found.group(1) if found else None


def save(name, page):
    for key in TOKENS:
        page = re.sub(r'(id="%s" value=")[^"]*(")' % key, r"\1REDACTED-%s\2" % key, page)
    with open(name, "w", encoding="utf-8") as out:
        out.write(page)


def headers_text(status, headers):
    lines = ["HTTP %d" % status]
    for key, value in headers.items():
        if key.lower() != "set-cookie":
            lines.append("%s: %s" % (key.lower(), value))
    return "\n".join(lines) + "\n"


evidence = {}
status, _, raw = request(SITE)
home = raw.decode("utf-8")
save("home.html", home)


def search(name, keyword="", per_page="50", org="0"):
    form = fields(home)
    form["ctrl_191017_163$txtSearchKeyword"] = keyword
    form["ctrl_191017_163$drdRecordPerPage"] = per_page
    form["ctrl_191017_163$drdDocOrg"] = org
    form["ctrl_191017_163$btnSearch"] = "Tìm kiếm"
    status, _, raw = request(SITE, form)
    page = raw.decode("utf-8")
    save(name, page)
    evidence[name] = {"status": status, "token_rotated": token(page, "__VIEWSTATE") != token(home, "__VIEWSTATE")}
    return page


def postback(name, previous, argument):
    form = fields(previous)
    form["__EVENTTARGET"] = "ctrl_191017_163$grvDocument"
    form["__EVENTARGUMENT"] = argument
    status, _, raw = request(SITE, form)
    page = raw.decode("utf-8")
    save(name, page)
    evidence[name] = {"status": status, "token_rotated": token(page, "__VIEWSTATE") != token(previous, "__VIEWSTATE")}
    return page


search("search-ai.html", "trí tuệ nhân tạo")
search("search-number-142-2026-ND-CP.html", "142/2026/NĐ-CP", "500")
search("search-number-367-QD-TTg.html", "367/QĐ-TTg", "500")
search("search-empty.html", "qzxjvkw trachuan", "500")
search("search-provincial-org-16134.html", "", "50", "16134")
first = search("search-truncated-page1.html", "ngân hàng", "500")
second = postback("search-truncated-page2.html", first, "Page$2")
postback("search-truncated-pagelast.html", second, "Page$Last")
postback("search-truncated-page3-stale-tokens.html", first, "Page$3")
for docid, name in (("218029", "detail-218029.html"), ("215941", "detail-215941.html"), ("999999999", "detail-missing.html")):
    status, _, raw = request(SITE + "?pageid=27160&docid=" + docid)
    save(name, raw.decode("utf-8"))
    evidence[name] = {"status": status}
status, head, _ = request("http://vanban.chinhphu.vn/")
with open("redirect-http.txt", "w", encoding="utf-8") as out:
    out.write(headers_text(status, head))
pdf = "https://datafiles.chinhphu.vn/cpp/files/vbpq/2026/4/142-2026-ndcp.signed.pdf"
status, head, _ = request(pdf, method="HEAD")
with open("pdf-142-headers.txt", "w", encoding="utf-8") as out:
    out.write(headers_text(status, head))
status, head, raw = request(pdf, headers={"Range": "bytes=0-1023"})
with open("pdf-142-head.bin", "wb") as out:
    out.write(raw)
evidence["pdf-142-head.bin"] = {"status": status, "content_range": head.get("Content-Range")}
status, head, raw = request("https://datafiles.chinhphu.vn/cpp/files/vbpq/2026/4/does-not-exist.pdf")
with open("file-missing.xml", "wb") as out:
    out.write(raw)
evidence["file-missing.xml"] = {"status": status, "content_type": head.get("Content-Type")}
with open("evidence.json", "w", encoding="utf-8") as out:
    json.dump(evidence, out, indent=1, sort_keys=True)
    out.write("\n")
PY
