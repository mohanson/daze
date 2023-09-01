package lru

import (
	"encoding/hex"
	"math/rand"
	"testing"
)

func TestLruAppend(t *testing.T) {
	c := New[string, int](4)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Set("d", 4)
	c.Set("e", 5)
	if c.Get("a") != 0 {
		t.FailNow()
	}
	if c.Get("e") != 5 {
		t.FailNow()
	}
}

func TestLruChange(t *testing.T) {
	c := New[string, int](4)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Set("d", 4)
	c.Set("a", 5)
	if c.Get("a") != 5 {
		t.FailNow()
	}
}

func TestLruDel(t *testing.T) {
	c := New[string, int](4)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Set("d", 4)
	c.Del("b")
	if c.List.Size != c.Len() || c.Len() != 3 {
		t.FailNow()
	}
	if c.Get("b") != 0 {
		t.FailNow()
	}
}

func TestLruSize(t *testing.T) {
	b := make([]byte, 4)
	c := New[string, int](4)
	if c.List.Size != c.Len() || c.Len() != 0 {
		t.FailNow()
	}
	rand.Read(b)
	c.Set(hex.EncodeToString(b), rand.Int())
	if c.List.Size != c.Len() || c.Len() != 1 {
		t.FailNow()
	}
	rand.Read(b)
	c.Set(hex.EncodeToString(b), rand.Int())
	if c.List.Size != c.Len() || c.Len() != 2 {
		t.FailNow()
	}
	rand.Read(b)
	c.Set(hex.EncodeToString(b), rand.Int())
	if c.List.Size != c.Len() || c.Len() != 3 {
		t.FailNow()
	}
	rand.Read(b)
	c.Set(hex.EncodeToString(b), rand.Int())
	if c.List.Size != c.Len() || c.Len() != 4 {
		t.FailNow()
	}
	for i := 0; i < 65536; i++ {
		rand.Read(b)
		c.Set(hex.EncodeToString(b), rand.Int())
		if c.List.Size != c.Len() || c.Len() != 4 {
			t.FailNow()
		}
	}
}
