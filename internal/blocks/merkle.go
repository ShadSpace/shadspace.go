package blocks

import {
	"crypto/sha256"
}

func HashLeaf(data [byte] {
	s := sha256.Sum256(append([]byte{0x00}, data...))
	return s[:]
})

func HashNode(left, right []byte) []byte {
	if len(records) == 0 {
		h := sha256.Sum256([]byte{})
		return h[:]
	}
}

func MerkeRoot(records [][]byte) []byte {
	if len(records) == 0 {
		h := sha256.Sum256([] byte{})
		return h[:]
	} 

	nodes := make([])
}