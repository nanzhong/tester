package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4"
)

var (
	pgDSN string
)

func TestMain(m *testing.M) {
	var status int
	defer func() {
		os.Exit(status)
	}()

	pgDSN = os.Getenv("PG_DSN")
	if pgDSN != "" {
		conn, err := pgx.Connect(context.Background(), pgDSN)
		if err != nil {
			fmt.Println(err)
			status = 1
			return
		}
		defer conn.Close(context.Background())

		cfg := conn.Config()
		testDB := fmt.Sprintf("tester_%d", time.Now().Unix())

		_, err = conn.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %s WITH OWNER = %s", testDB, cfg.User))
		if err != nil {
			fmt.Println(err)
			status = 1
			return
		}
		defer func() {
			_, err := conn.Exec(context.Background(), fmt.Sprintf("DROP DATABASE %s", testDB))
			if err != nil {
				fmt.Println(err)
				status = 1
				return
			}
		}()

		pgDSN = fmt.Sprintf("postgres://%s:%s@%s:%d/%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, testDB)
		os.Setenv("PG_DSN", pgDSN)
	}

	status = m.Run()
}
