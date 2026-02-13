package db

import (
	"database/sql"
	"strings"

	"github.com/rakib/ai-cart-sms-tester/internal/model"
	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(dataSourceName string) error {
	var err error
	// Enable WAL mode for better concurrency and busy timeout
	if !strings.Contains(dataSourceName, "?") {
		dataSourceName += "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	DB, err = sql.Open("sqlite", dataSourceName)
	if err != nil {
		return err
	}

	if err = DB.Ping(); err != nil {
		return err
	}

	return createTables()
}

func createTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS sms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sender TEXT,
		recipient TEXT,
		body TEXT,
		is_read BOOLEAN DEFAULT FALSE,
		created_at DATETIME
	);
	`
	_, err := DB.Exec(query)
	// Migration for existing tables (ignore error if column exists)
	if err == nil {
		_, _ = DB.Exec(`ALTER TABLE sms ADD COLUMN is_read BOOLEAN DEFAULT FALSE;`)
	}
	return err
}

func SaveSMS(sms *model.SMS) (int64, error) {
	query := `INSERT INTO sms (sender, recipient, body, is_read, created_at) VALUES (?, ?, ?, ?, ?)`
	res, err := DB.Exec(query, sms.From, sms.To, sms.Body, sms.IsRead, sms.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func GetMessages(page, limit int, search string) ([]model.SMS, error) {
	offset := (page - 1) * limit
	query := `SELECT id, sender, recipient, body, is_read, created_at FROM sms`
	var args []interface{}

	if search != "" {
		query += ` WHERE sender LIKE ? OR recipient LIKE ? OR body LIKE ?`
		searchParam := "%" + search + "%"
		args = append(args, searchParam, searchParam, searchParam)
	}

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.SMS
	for rows.Next() {
		var m model.SMS
		if err := rows.Scan(&m.ID, &m.From, &m.To, &m.Body, &m.IsRead, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

func GetStats(search string) (total int, unread int, err error) {
	queryTotal := "SELECT COUNT(*) FROM sms"
	queryUnread := "SELECT COUNT(*) FROM sms WHERE is_read = FALSE"
	var args []interface{}

	if search != "" {
		where := " WHERE (sender LIKE ? OR recipient LIKE ? OR body LIKE ?)"
		queryTotal = "SELECT COUNT(*) FROM sms" + where
		// For unread with search, we need AND
		queryUnread = "SELECT COUNT(*) FROM sms WHERE is_read = FALSE AND (sender LIKE ? OR recipient LIKE ? OR body LIKE ?)"

		searchParam := "%" + search + "%"
		args = append(args, searchParam, searchParam, searchParam)
	}

	err = DB.QueryRow(queryTotal, args...).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	err = DB.QueryRow(queryUnread, args...).Scan(&unread)
	return total, unread, err
}

func MarkAsRead(id int64) error {
	_, err := DB.Exec("UPDATE sms SET is_read = TRUE WHERE id = ?", id)
	return err
}

func DeleteAllMessages() error {
	_, err := DB.Exec("DELETE FROM sms")
	return err
}
