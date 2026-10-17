package main

import (
	"strconv"
	"testing"
)

func TestDatabaseWorkerLimitAndDefault(t *testing.T) {
	c, err := flags(nil)
	if err != nil || c.workers != 4 {
		t.Fatalf("default bounded worker configuration: %#v %v", c, err)
	}
	for _, workers := range []int{-1, 0, 1, 4, 8, 9} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			c, err := flags([]string{"--workers", strconv.Itoa(workers)})
			valid := workers >= 1 && workers <= 8
			if (err == nil) != valid || valid && c.workers != workers {
				t.Fatalf("worker limit %d: got %d, error %v", workers, c.workers, err)
			}
		})
	}
}
