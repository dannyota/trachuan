# Account mail

The mail outbox for [email accounts](accounts.md): classes, the sender and mail
content. Numbers are in [budgets](budgets.md#accounts).

## Mail outbox

Classes share the global caps: `signup` (registration and account-exists mail)
may use at most the signup share of the hourly and monthly budget; `recovery`
(reset, add password) and `notice` (security notices) use the rest, and a
security notice is always enqueued in the transaction of its change, outside the
row cap. Notices stay bounded because security changes are: each account may
make at most the number of security changes per day in budgets (reset
completions have their own allowance, so signed-in changes cannot block mailbox
recovery), and a change beyond it is refused with 429 before anything is
written, so every committed change has exactly one notice and no row is ever
replaced. One sender claims a due row by setting `claimed_until` and
incrementing `attempts` in a committed statement, then sends: SMTP to OCI Email
Delivery with mandatory STARTTLS, certificate verification for the configured
host and AUTH only after TLS, under one deadline per message. Success sets
`sent_at` and clears the payload; a failure sets `next_attempt_at`; a crash
leaves the row to be claimed again after `claimed_until`. A message whose
outcome was uncertain may be delivered twice. After the attempt bound or
`expires_at` the payload is cleared. A link in mail is
`SERVER_PUBLIC_URL + "/link#" + token`: the token stays in the fragment, so it
never reaches server or proxy logs or a `Referer`, and the web sends it only in
a request body. Outside a loopback deployment (the rule that admits the fake
Google issuer) the server refuses to start without SMTP settings;
`SERVER_MAIL_CAPTURE_DIR`, which writes each message to a 0600 file, is allowed
only in a loopback deployment. Mail is from `no-reply@trachuan.vn`, Vietnamese
then English, plain text. Kinds: `registration`, `account_exists`,
`add_password`, `reset`, `password_changed`, `totp_enrolled`, `totp_removed`,
`recovery_codes_replaced`. Logs carry kind, row id and result, never the address
or link.
