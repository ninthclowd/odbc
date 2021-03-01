package odbc

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"
)

var (
	nsdsn  = flag.String("nsdsn", "NetSuite", "")
	nsuid  = flag.String("nsuid", "", "")
	nspwd  = flag.String("nspwd", "", "")
	nsacct = flag.String("nsacct", "", "")
	nsrole = flag.String("nsrole", "3", "")
)

func TestNSExecContextTimeout(t *testing.T) {
	connStr := fmt.Sprintf(
		"DSN=%s;Uid=%s;Pwd=%s;CustomProperties=AccountID=%s;RoleID=%s",
		*nsdsn,
		*nsuid,
		*nspwd,
		*nsacct,
		*nsrole)
	t.Log(connStr)
	db, err := sql.Open("odbc", connStr)
	if err != nil {
		t.Skip("skipping ns tests")
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if _, qErr := db.ExecContext(ctx, "SELECT * FROM TRANSACTIONS"); qErr == nil {
		t.Fatal("expected an error to be returned")
	} else if !errors.Is(qErr, context.DeadlineExceeded) {
		t.Fatalf("expected a context canceled error. got: %s", qErr.Error())
	}
	if time.Since(start).Seconds() > 2 {
		t.Fatal("exec should have been canceled after 1 second")
	}

	if _, qErr := db.ExecContext(ctx, "SELECT 1"); qErr == nil {
		t.Fatal("expected an error to be returned for subsequent exec on expired context")
	} else if !errors.Is(qErr, context.DeadlineExceeded) {
		t.Fatalf("expected a context canceled error. got: %s", qErr.Error())
	}

	if _, qErr := db.ExecContext(context.Background(), "SELECT 1"); qErr != nil {
		t.Fatalf("exec on a fresh context should execute without error.  Got: %s", qErr.Error())
	}

	//wait for the query to finish cancelling in the background and for the connection to close
	time.Sleep(3 * time.Second)

	db.Close()
}

func TestNSPing(t *testing.T) {
	connStr := fmt.Sprintf(
		"DSN=%s;Uid=%s;Pwd=%s;CustomProperties=AccountID=%s;RoleID=%s",
		*nsdsn,
		*nsuid,
		*nspwd,
		*nsacct,
		*nsrole)
	t.Log(connStr)
	db, err := sql.Open("odbc", connStr)

	if err != nil {
		t.Fatal(err)
	}

	c, _ := db.Conn(context.Background())

	if err := c.PingContext(context.Background()); err != nil {
		t.Fatalf("did not expect an error from ping. got %s", err.Error())
	}

	c.Close()

	if err := c.PingContext(context.Background()); err == nil {
		t.Fatalf("expected ping to fail after being closed")
	}

}
