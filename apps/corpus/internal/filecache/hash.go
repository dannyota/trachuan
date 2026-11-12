package filecache

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"hash"
)

// Hashes identifies bytes independently of source and collection time.
type Hashes struct{ SHA256, MD5, ETag16, ETag5 string }

// Hasher computes SHA-256, MD5 and the supported S3 multipart ETags in one pass.
type Hasher struct {
	sha, md5        hash.Hash
	parts16, parts5 multipart
}
type multipart struct {
	partSize, size int64
	part           hash.Hash
	digests        []byte
}

// NewHasher constructs an empty streaming hash calculator.
func NewHasher() *Hasher {
	return &Hasher{sha: sha256.New(), md5: md5.New(), parts16: newMultipart(16 << 20), parts5: newMultipart(5 << 20)}
}
func newMultipart(size int64) multipart {
	return multipart{partSize: size, part: md5.New()}
}

// Write consumes bytes without retaining their contents.
func (h *Hasher) Write(p []byte) (int, error) {
	_, _ = h.sha.Write(p)
	_, _ = h.md5.Write(p)
	h.parts16.write(p)
	h.parts5.write(p)
	return len(p), nil
}
func (m *multipart) write(p []byte) {
	for len(p) > 0 {
		n := min(int64(len(p)), m.partSize-m.size)
		_, _ = m.part.Write(p[:n])
		m.size += n
		p = p[n:]
		if m.size == m.partSize {
			m.digests = append(m.digests, m.part.Sum(nil)...)
			m.part.Reset()
			m.size = 0
		}
	}
}
func (m *multipart) etag() string {
	n := len(m.digests) / md5.Size
	sum := md5.New()
	_, _ = sum.Write(m.digests)
	if m.size > 0 {
		_, _ = sum.Write(m.part.Sum(nil))
		n++
	}
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf("%x-%d", sum.Sum(nil), n)
}

// Hashes returns hashes for all bytes written so far.
func (h *Hasher) Hashes() Hashes {
	return Hashes{SHA256: fmt.Sprintf("%x", h.sha.Sum(nil)), MD5: fmt.Sprintf("%x", h.md5.Sum(nil)), ETag16: h.parts16.etag(), ETag5: h.parts5.etag()}
}

// Match returns the source hash form that equals this candidate.
func (h Hashes) Match(etag string) string {
	if etag == h.MD5 && len(etag) == 32 {
		return "etag_md5"
	}
	if etag != "" && (etag == h.ETag16 || etag == h.ETag5) {
		return "etag_multipart"
	}
	return ""
}
