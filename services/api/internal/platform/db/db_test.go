package db

import (
	"context"
	"testing"
)

func TestDBNilPing(t *testing.T) {
	var d *DB
	err := d.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error pinging nil DB, got nil")
	}

	d2 := &DB{Pool: nil}
	err2 := d2.Ping(context.Background())
	if err2 == nil {
		t.Fatal("expected error pinging DB with nil Pool, got nil")
	}
}

func TestDBInvalidConfig(t *testing.T) {
	_, err := Connect(context.Background(), Config{
		URL: "invalid://url",
	})
	if err == nil {
		t.Fatal("expected error with invalid database URL, got nil")
	}
}
