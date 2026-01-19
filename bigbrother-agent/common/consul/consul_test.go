package consul

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func init() {
	os.Setenv("CONSUL_HTTP_HOST", "10.224.6.108")
}

func TestLookup(t *testing.T) {
	psm := "data.ti.metaservice"

	IsBoeTest = true
	for i := 0; i < 10; i++ {
		eps, err := Lookup(psm)
		if err != nil {
			t.Fatal(err)
		}
		if len(eps) != 1 {
			t.Fail()
		}
		fmt.Println(eps)
		time.Sleep(time.Second)
	}

	fmt.Println("------------")

	IsBoeTest = false
	for i := 0; i < 10; i++ {
		eps, err := Lookup(psm)
		if err != nil {
			t.Fatal(err)
		}
		if len(eps) != 2 {
			t.Fatal()
		}
		fmt.Println(eps)
		time.Sleep(time.Second)
	}
}
