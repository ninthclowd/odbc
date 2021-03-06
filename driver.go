// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package odbc implements database/sql driver to access data via odbc interface.
//
package odbc

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"unsafe"

	"github.com/ninthclowd/odbc/api"
	"go.uber.org/atomic"
)

var drv Driver

type Driver struct {
	Stats
	h       api.SQLHENV // environment handle
	initErr error
}

func initDriver() error {
	//initialize allocation counters
	drv.Stats = Stats{
		EnvCount:  atomic.NewInt32(0),
		ConnCount: atomic.NewInt32(0),
		StmtCount: atomic.NewInt32(0),
	}
	//Allocate environment handle
	var out api.SQLHANDLE
	in := api.SQLHANDLE(api.SQL_NULL_HANDLE)
	ret := api.SQLAllocHandle(api.SQL_HANDLE_ENV, in, &out)
	if IsError(ret) {
		return NewError("SQLAllocHandle", api.SQLHENV(in))
	}
	drv.h = api.SQLHENV(out)
	drv.Stats.EnvCount.Inc()

	// will use ODBC v3
	ret = api.SQLSetEnvUIntPtrAttr(drv.h, api.SQL_ATTR_ODBC_VERSION, api.SQL_OV_ODBC3, 0)
	if IsError(ret) {
		defer releaseHandle(drv.h)
		return NewError("SQLSetEnvUIntPtrAttr", drv.h)
	}

	//TODO: find a way to make this attribute changeable at runtime
	//Enable connection pooling
	ret = api.SQLSetEnvUIntPtrAttr(drv.h, api.SQL_ATTR_CONNECTION_POOLING, api.SQL_CP_ONE_PER_HENV, api.SQL_IS_UINTEGER)
	if IsError(ret) {
		defer releaseHandle(drv.h)
		return NewError("SQLSetEnvUIntPtrAttr", drv.h)
	}

	//Set relaxed connection pool matching
	ret = api.SQLSetEnvUIntPtrAttr(drv.h, api.SQL_ATTR_CP_MATCH, api.SQL_CP_RELAXED_MATCH, api.SQL_IS_UINTEGER)
	if IsError(ret) {
		defer releaseHandle(drv.h)
		return NewError("SQLSetEnvUIntPtrAttr", drv.h)
	}

	//TODO: it would be nice if we could call "drv.SetMaxIdleConns(0)" here but from the docs it looks like
	//the user must call this function after db.Open

	return nil
}

//TODO(ninthclowd): this is not part of the driver.Driver interface and will never be called by a consumer
func (d *Driver) Close() error {
	// TODO(brainman): who will call (*Driver).Close (to dispose all opened handles)?
	h := d.h
	d.h = api.SQLHENV(api.SQL_NULL_HENV)
	return releaseHandle(h)
}

func init() {
	err := initDriver()
	if err != nil {
		drv.initErr = err
	}
	sql.Register("odbc", &drv)
}

// implement driver.Driver
func (d *Driver) Open(name string) (driver.Conn, error) {
	return d.open(name, context.Background())
}

func (d *Driver) open(name string, dialContext context.Context) (driver.Conn, error) {
	if d.initErr != nil {
		return nil, d.initErr
	}
	//TODO(ninthclowd): return early if dialContext expires while connecting
	var out api.SQLHANDLE
	ret := api.SQLAllocHandle(api.SQL_HANDLE_DBC, api.SQLHANDLE(d.h), &out)
	if IsError(ret) {
		return nil, NewError("SQLAllocHandle", d.h)
	}
	h := api.SQLHDBC(out)
	drv.Stats.ConnCount.Inc()

	b := api.StringToUTF16(name)
	ret = api.SQLDriverConnect(h, 0,
		(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQL_NTS,
		nil, 0, nil, api.SQL_DRIVER_NOPROMPT)
	if IsError(ret) {
		defer releaseHandle(h)
		return nil, NewError("SQLDriverConnect", h)
	}
	isAccess := strings.Contains(strings.ToUpper(strings.Replace(name, " ", "", -1)), accessDriverSubstr)
	return &Conn{h: h, isMSAccessDriver: isAccess, bad: atomic.NewBool(false), closingInBG: atomic.NewBool(false)}, nil
}
