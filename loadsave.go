package meddler

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx"
)

type dbErr struct {
	msg string
	err error
}

func (err *dbErr) Error() string {
	return fmt.Sprintf("%s: %v", err.msg, err.err)
}

// DriverErr returns the original error as returned by the database driver
// if the error comes from the driver, with the second value set to true.
// Otherwise, it returns err itself with false as second value.
func DriverErr(err error) (error, bool) {
	if dbe, ok := err.(*dbErr); ok {
		return dbe.err, true
	}
	return err, false
}

// DB is a generic pgx database interface, matching *pgx.Conn, *pgx.ConnPool
// and *pgx.Tx.
type DB interface {
	Exec(query string, args ...interface{}) (pgx.CommandTag, error)
	Query(query string, args ...interface{}) (*pgx.Rows, error)
	QueryRow(query string, args ...interface{}) *pgx.Row
}

// Load loads a record using a query for the primary key field.
// Returns sql.ErrNoRows if not found.
func Load(db DB, table string, dst interface{}, pk int64) error {
	columns, err := d.ColumnsQuoted(dst, true)
	if err != nil {
		return err
	}

	// make sure we have a primary key field
	pkName, _, err := d.PrimaryKey(dst)
	if err != nil {
		return err
	}
	if pkName == "" {
		return fmt.Errorf("meddler.Load: no primary key field found")
	}

	// run the query
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s = %s", columns, d.quoted(table), d.quoted(pkName), d.Placeholder)

	rows, err := runQuery(db, q, pk)
	if err != nil {
		return &dbErr{msg: "meddler.Load: DB error in Query", err: err}
	}

	// scan the row
	return d.ScanRow(rows, dst)
}

// Insert performs an INSERT query for the given record.
// If the record has a primary key flagged, it must be zero, and it
// will be set to the newly-allocated primary key value from the database
// as returned by LastInsertId.
func Insert(db DB, table string, src interface{}) error {
	pkName, pkValue, err := d.PrimaryKey(src)
	if err != nil {
		return err
	}
	if pkName != "" && pkValue != 0 {
		return fmt.Errorf("meddler.Insert: primary key must be zero")
	}

	// gather the query parts
	namesPart, err := d.ColumnsQuoted(src, false)
	if err != nil {
		return err
	}
	valuesPart, err := d.PlaceholdersString(src, false)
	if err != nil {
		return err
	}
	values, err := d.Values(src, false)
	if err != nil {
		return err
	}

	// run the query
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.quoted(table), namesPart, valuesPart)
	if d.UseReturningToGetID && pkName != "" {
		q += " RETURNING " + d.quoted(pkName)
		var newPk int64

		row, err := runQueryRow(db, q, values...)
		if err != nil {
			return err
		}
		err = row.Scan(&newPk)
		if err != nil {
			return &dbErr{msg: "meddler.Insert: DB error in QueryRow", err: err}
		}
		if err = d.SetPrimaryKey(src, newPk); err != nil {
			return fmt.Errorf("meddler.Insert: Error saving updated pk: %v", err)
		}
	} else if pkName != "" {
		result, err := runExec(db, q, values...)
		if err != nil {
			return &dbErr{msg: "meddler.Insert: DB error in Exec", err: err}
		}

		// save the new primary key
		newPk, err := result.LastInsertId()
		if err != nil {
			return &dbErr{msg: "meddler.Insert: DB error getting new primary key value", err: err}
		}
		if err = d.SetPrimaryKey(src, newPk); err != nil {
			return fmt.Errorf("meddler.Insert: Error saving updated pk: %v", err)
		}
	} else {
		// no primary key, so no need to lookup new value
		if _, err := runExec(db, q, values...); err != nil {
			return &dbErr{msg: "meddler.Insert: DB error in Exec", err: err}
		}
	}

	return nil
}

// Update performs and UPDATE query for the given record.
// The record must have an integer primary key field that is non-zero,
// and it will be used to select the database row that gets updated.
func Update(db DB, table string, src interface{}) error {
	// gather the query parts
	names, err := d.Columns(src, false)
	if err != nil {
		return err
	}
	placeholders, err := d.Placeholders(src, false)
	if err != nil {
		return err
	}
	values, err := d.Values(src, false)
	if err != nil {
		return err
	}

	// form the column=placeholder pairs
	var pairs []string
	for i := 0; i < len(names) && i < len(placeholders); i++ {
		pair := fmt.Sprintf("%s=%s", d.quoted(names[i]), placeholders[i])
		pairs = append(pairs, pair)
	}

	pkName, pkValue, err := d.PrimaryKey(src)
	if err != nil {
		return err
	}
	if pkName == "" {
		return fmt.Errorf("meddler.Update: no primary key field")
	}
	if pkValue < 1 {
		return fmt.Errorf("meddler.Update: primary key must be an integer > 0")
	}
	ph := d.placeholder(len(placeholders) + 1)

	// run the query
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s=%s", d.quoted(table),
		strings.Join(pairs, ","),
		d.quoted(pkName), ph)
	values = append(values, pkValue)

	if _, err := runExec(db, q, values...); err != nil {
		return &dbErr{msg: "meddler.Update: DB error in Exec", err: err}
	}

	return nil
}

// Save performs an INSERT or an UPDATE, depending on whether or not
// a primary keys exists and is non-zero.
func Save(db DB, table string, src interface{}) error {
	pkName, pkValue, err := d.PrimaryKey(src)
	if err != nil {
		return err
	}
	if pkName != "" && pkValue != 0 {
		return d.Update(db, table, src)
	} else {
		return d.Insert(db, table, src)
	}
}

// QueryOne performs the given query with the given arguments, scanning a
// single row of results into dst. Returns sql.ErrNoRows if there was no
// result row.
func QueryRow(db DB, dst interface{}, query string, args ...interface{}) error {
	// perform the query
	rows, err := runQuery(db, query, args...)
	if err != nil {
		return err
	}

	// gather the result
	return d.ScanRow(rows, dst)
}

// QueryAll performs the given query with the given arguments, scanning
// all results rows into dst.
func QueryAll(db DB, dst interface{}, query string, args ...interface{}) error {
	// perform the query
	rows, err := runQuery(db, query, args...)
	if err != nil {
		return err
	}

	// gather the results
	return d.ScanAll(rows, dst)
}

func runQuery(db DB, q string, args ...interface{}) (*sql.Rows, error) {
	if StmtCacheFunc != nil {
		stmt, err := StmtCacheFunc(db, q)
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			return stmt.Query(args...)
		}
	}
	return db.Query(q, args...)
}

func runQueryRow(db DB, q string, args ...interface{}) (*sql.Row, error) {
	if StmtCacheFunc != nil {
		stmt, err := StmtCacheFunc(db, q)
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			return stmt.QueryRow(args...), nil
		}
	}
	return db.QueryRow(q, args...), nil
}

func runExec(db DB, q string, args ...interface{}) (sql.Result, error) {
	if StmtCacheFunc != nil {
		stmt, err := StmtCacheFunc(db, q)
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			return stmt.Exec(args...)
		}
	}
	return db.Exec(q, args...)
}
