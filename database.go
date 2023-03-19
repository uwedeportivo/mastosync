package main

import (
	"database/sql"
	"os"
	"time"
)

type DAO struct {
	db *sql.DB
}

type Toot struct {
	RSSGUID   string
	MastID    string
	Timestamp time.Time
}

const createTableSQL string = `CREATE TABLE mastosync (
		   "rssguid" TEXT NOT NULL PRIMARY KEY,		
		   "mastid" TEXT,
           "timestamp" TEXT 
	    );`
const insertTableSQL string = "INSERT INTO mastosync (rssguid, mastid, timestamp) VALUES (?, ?, ?)"
const selectTableSQL string = "SELECT mastid, timestamp FROM mastosync WHERE rssguid=?"

func (dao *DAO) RecordSync(rssguid, mastid string, ts time.Time) error {
	toot, err := dao.FindToot(rssguid)
	if err != nil {
		return err
	}
	if toot != nil {
		return nil
	}
	_, err = dao.db.Exec(insertTableSQL, rssguid, mastid, ts)
	return err
}

func (dao *DAO) FindToot(rssguid string) (*Toot, error) {
	rows, err := dao.db.Query(selectTableSQL, rssguid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var toot Toot
	var ts string
	for rows.Next() {
		err = rows.Scan(&toot.MastID, &ts)
		if err != nil {
			return nil, err
		}
		toot.RSSGUID = rssguid
		t, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", ts)
		if err != nil {
			return nil, err
		}
		toot.Timestamp = t
		return &toot, nil
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func CreateDB(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	err = file.Close()
	if err != nil {
		return err
	}

	err = os.Chmod(path, 0600)
	if err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}

	stmt, err := db.Prepare(createTableSQL) // Prepare SQL Statement
	if err != nil {
		return err
	}
	_, err = stmt.Exec() // Execute SQL Statements
	if err != nil {
		return err
	}
	return nil
}

func OpenDB(path string) (*DAO, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	return &DAO{
		db: db,
	}, nil
}
