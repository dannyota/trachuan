package build

import "encoding/json"

type inheritanceRow struct {
	Name        string  `json:"name"`
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
	Floor       float64 `json:"floor"`
	Status      string  `json:"status"`
}

func (r inheritanceRow) MarshalJSON() ([]byte, error) {
	type row inheritanceRow
	var value *float64
	if r.Denominator > 0 {
		v := float64(r.Numerator) / float64(r.Denominator)
		value = &v
	}
	return json.Marshal(struct {
		row
		Value *float64 `json:"value"`
	}{row: row(r), Value: value})
}

type inheritanceGate struct {
	Articles int               `json:"articles"`
	Status   string            `json:"status"`
	Reason   string            `json:"reason,omitempty"`
	Rows     [3]inheritanceRow `json:"rows"`
}

func finishInheritanceGate(gate *inheritanceGate) {
	gate.Status = "pass"
	for i := range gate.Rows {
		row := &gate.Rows[i]
		row.Status = "pass"
		if gate.Articles < 100 || row.Denominator == 0 {
			row.Status = "unavailable"
		} else if float64(row.Numerator)/float64(row.Denominator) < row.Floor {
			row.Status = "fail"
		}
		if row.Status == "unavailable" {
			gate.Status = "unavailable"
		} else if row.Status == "fail" && gate.Status == "pass" {
			gate.Status = "fail"
		}
	}
	if gate.Articles < 100 {
		gate.Reason = "fewer_than_100_articles"
	} else if gate.Status == "unavailable" {
		gate.Reason = "no_predictions"
	} else if gate.Status == "fail" {
		gate.Reason = "floor_failed"
	}
}

func validInheritanceGate(gate inheritanceGate) bool {
	expected := [3]inheritanceRow{{Name: "level_2", Floor: 0.95}, {Name: "level_3", Floor: 0.90}, {Name: "deepest", Floor: 0.85}}
	if gate.Articles < 0 {
		return false
	}
	for i, row := range gate.Rows {
		if row.Name != expected[i].Name || row.Floor != expected[i].Floor || row.Numerator < 0 || row.Denominator < row.Numerator {
			return false
		}
	}
	original := gate
	gate.Reason = ""
	finishInheritanceGate(&gate)
	return gate == original
}
