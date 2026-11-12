package filecache

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestHashes_MD5AndMultipart(t *testing.T) {
	for _, size := range []int{0, 3, 5 << 20, 5<<20 + 1, 16<<20 + 1, 33 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := bytes.Repeat([]byte{'x'}, size)
			h := NewHasher()
			for at := 0; at < len(data); {
				end := min(at+719, len(data))
				_, _ = h.Write(data[at:end])
				at = end
			}
			got := h.Hashes()
			if got.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || got.MD5 != fmt.Sprintf("%x", md5.Sum(data)) {
				t.Fatal(got)
			}
			for _, part := range []int{5 << 20, 16 << 20} {
				expected := ""
				if size > part {
					var hashes []byte
					n := 0
					for at := 0; at < size; at += part {
						hash := md5.Sum(data[at:min(at+part, size)])
						hashes = append(hashes, hash[:]...)
						n++
					}
					expected = fmt.Sprintf("%x-%d", md5.Sum(hashes), n)
				}
				actual := got.ETag5
				if part == 16<<20 {
					actual = got.ETag16
				}
				if actual != expected {
					t.Fatalf("part=%d got=%s want=%s", part, actual, expected)
				}
			}
		})
	}
}
