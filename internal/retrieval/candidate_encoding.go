package retrieval

import (
	"encoding/binary"
	"math"
)

func encodeCandidates(groups [2]ArmGroup) []byte {
	size := 14
	for _, g := range groups {
		size += 28 * (len(g.RawANN) + len(g.Dense) + len(g.Lexical))
	}
	raw := make([]byte, 0, size)
	for _, g := range groups {
		flag := byte(0)
		if g.Fallback {
			flag = 1
		}
		raw = append(raw, flag)
		for _, list := range [][]Candidate{g.RawANN, g.Dense, g.Lexical} {
			length := uint16(len(list))
			if list == nil {
				length = math.MaxUint16
			}
			raw = binary.LittleEndian.AppendUint16(raw, length)
			for _, c := range list {
				raw = binary.LittleEndian.AppendUint64(raw, uint64(c.ChunkID))
				raw = binary.LittleEndian.AppendUint32(raw, uint32(c.Rank))
				raw = binary.LittleEndian.AppendUint64(raw, math.Float64bits(c.Cosine))
				raw = binary.LittleEndian.AppendUint64(raw, math.Float64bits(c.BM25))
			}
		}
	}
	return raw
}

func decodeCandidates(raw []byte) ([2]ArmGroup, bool) {
	var groups [2]ArmGroup
	for i := range groups {
		if len(raw) < 1 || raw[0] > 1 {
			return groups, false
		}
		groups[i].Fallback = raw[0] == 1
		raw = raw[1:]
		for _, list := range []*[]Candidate{&groups[i].RawANN, &groups[i].Dense, &groups[i].Lexical} {
			if len(raw) < 2 {
				return groups, false
			}
			length := int(binary.LittleEndian.Uint16(raw))
			raw = raw[2:]
			if length == math.MaxUint16 {
				continue
			}
			if length > 200 || length > len(raw)/28 {
				return groups, false
			}
			*list = make([]Candidate, length)
			for j := range *list {
				(*list)[j] = Candidate{
					ChunkID: int64(binary.LittleEndian.Uint64(raw)),
					Rank:    int(binary.LittleEndian.Uint32(raw[8:])),
					Cosine:  math.Float64frombits(binary.LittleEndian.Uint64(raw[12:])),
					BM25:    math.Float64frombits(binary.LittleEndian.Uint64(raw[20:])),
				}
				raw = raw[28:]
			}
		}
	}
	return groups, len(raw) == 0
}
