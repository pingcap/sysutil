package diagnostics

import (
	"fmt"
	"testing"
)

func TestGetLoadInfo(t *testing.T) {
	info, err := getLoadInfo()
	checkErrIsNil(t, err)

	for _, v := range info {
		for _, item := range v.Pairs {
			fmt.Printf("%v, %v, %v, %v\n", v.Tp, v.Name, item.Key, item.Value)
		}
	}
}

func checkErrIsNil(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
