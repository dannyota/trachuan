package build

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"

	"danny.vn/trachuan/internal/frameworks/relate"
)

func readTitles(path string) (map[relate.Key]string, error) {
	titles := map[relate.Key]string{}
	data, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return titles, nil
	}
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(data))
	header, err := r.Read()
	if err != nil {
		return nil, hide("read titles header", err)
	}
	if !reflect.DeepEqual(header, []string{"code", "version", "citation", "title"}) {
		return nil, fmt.Errorf("invalid titles header")
	}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			return titles, nil
		}
		if err != nil {
			return nil, hide("read titles row", err)
		}
		key := relate.Key{Code: row[0], Version: row[1], Citation: row[2]}
		if _, exists := titles[key]; exists || key.Code == "" || key.Version == "" || key.Citation == "" {
			return nil, fmt.Errorf("invalid or duplicate title key")
		}
		titles[key] = row[3]
	}
}
