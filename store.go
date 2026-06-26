package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

type Alert struct {
	ID          int64
	UserID      int64
	Coin        string
	AlertType   string
	Threshold   float64
	Direction   string
	CreatedAt   string
	Active      bool
	TriggeredAt sql.NullString
}

type Settings struct {
	UserID      int64
	DailyDigest bool
	DigestTime  string
	Timezone    string
}

func InitDB(path string) (*sql.DB, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}

	schema := `
CREATE TABLE IF NOT EXISTS users (
	id         BIGINT PRIMARY KEY,
	username   TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	is_premium BOOLEAN DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS alerts (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      BIGINT NOT NULL,
	coin         TEXT NOT NULL,
	target_price REAL NOT NULL,
	direction    TEXT NOT NULL,
	created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
	active       BOOLEAN DEFAULT TRUE,
	FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS settings (
	user_id      INTEGER PRIMARY KEY,
	daily_digest BOOLEAN DEFAULT TRUE,
	digest_time  TEXT DEFAULT '08:00',
	timezone     TEXT DEFAULT 'UTC'
);
`
	if _, err := d.Exec(schema); err != nil {
		d.Close()
		return nil, err
	}
	if err := migrateAlerts(d); err != nil {
		d.Close()
		return nil, err
	}
	if err := migrateUsers(d); err != nil {
		d.Close()
		return nil, err
	}
	db = d
	return d, nil
}

func migrateAlerts(d *sql.DB) error {
	rows, err := d.Query("PRAGMA table_info(alerts)")
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		existing[name] = true
	}
	rows.Close()

	additions := []struct{ name, def string }{
		{"alert_type", "TEXT NOT NULL DEFAULT 'price'"},
		{"threshold", "REAL"},
		{"triggered_at", "DATETIME"},
	}
	for _, a := range additions {
		if existing[a.name] {
			continue
		}
		if _, err := d.Exec(fmt.Sprintf("ALTER TABLE alerts ADD COLUMN %s %s", a.name, a.def)); err != nil {
			return err
		}
	}
	if _, err := d.Exec(
		"UPDATE alerts SET threshold = target_price WHERE threshold IS NULL",
	); err != nil {
		return err
	}
	return nil
}

func init() {
	// migration runner stitched after InitDB returns
}

func migrateUsers(d *sql.DB) error {
	rows, err := d.Query("PRAGMA table_info(users)")
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		existing[name] = true
	}
	rows.Close()

	if !existing["premium_expires_at"] {
		if _, err := d.Exec("ALTER TABLE users ADD COLUMN premium_expires_at DATETIME"); err != nil {
			return err
		}
	}
	return nil
}

func CloseDB() {
	if db != nil {
		db.Close()
	}
}

func EnsureUser(userID int64, username string) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO users (id, username) VALUES (?, ?)",
		userID, username,
	)
	return err
}

func IsUserPremium(userID int64) (bool, error) {
	var isPremium bool
	var expiresAt sql.NullString
	err := db.QueryRow("SELECT is_premium, premium_expires_at FROM users WHERE id = ?", userID).Scan(&isPremium, &expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !isPremium {
		return false, nil
	}
	if expiresAt.Valid && expiresAt.String != "" {
		t, err := time.Parse("2006-01-02 15:04:05", expiresAt.String)
		if err == nil && time.Now().UTC().After(t) {
			db.Exec("UPDATE users SET is_premium = FALSE, premium_expires_at = NULL WHERE id = ?", userID)
			return false, nil
		}
	}
	return true, nil
}

func GetUserAlertCount(userID int64) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM alerts WHERE user_id = ? AND active = TRUE",
		userID,
	).Scan(&count)
	return count, err
}

func CreateAlert(userID int64, coin, alertType, direction string, threshold float64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO alerts (user_id, coin, target_price, direction, alert_type, threshold, active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		userID, coin, threshold, direction, alertType, threshold,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanAlert(scanner interface{ Scan(...any) error }) (Alert, error) {
	var a Alert
	err := scanner.Scan(
		&a.ID, &a.UserID, &a.Coin, &a.AlertType, &a.Threshold,
		&a.Direction, &a.CreatedAt, &a.Active, &a.TriggeredAt,
	)
	return a, err
}

const alertColumns = `id, user_id, coin, alert_type, threshold, direction, created_at, active, triggered_at`

func GetUserAlerts(userID int64) ([]Alert, error) {
	rows, err := db.Query(
		"SELECT "+alertColumns+" FROM alerts WHERE user_id = ? AND active = TRUE ORDER BY id",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func GetActiveAlerts() ([]Alert, error) {
	rows, err := db.Query(
		"SELECT " + alertColumns + " FROM alerts WHERE active = TRUE ORDER BY id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func DeleteAlert(userID, alertID int64) error {
	res, err := db.Exec("DELETE FROM alerts WHERE id = ? AND user_id = ?", alertID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alert %d not found or not owned by you", alertID)
	}
	return nil
}

func SetUserPremium(userID int64, premium bool) error {
	if premium {
		expiresAt := time.Now().UTC().AddDate(0, 1, 0).Format("2006-01-02 15:04:05")
		_, err := db.Exec("UPDATE users SET is_premium = ?, premium_expires_at = ? WHERE id = ?", premium, expiresAt, userID)
		return err
	}
	_, err := db.Exec("UPDATE users SET is_premium = ?, premium_expires_at = NULL WHERE id = ?", premium, userID)
	return err
}

func SetUserPremiumOnce(userID int64) error {
	expiresAt := "9999-12-31 23:59:59"
	_, err := db.Exec("UPDATE users SET is_premium = TRUE, premium_expires_at = ? WHERE id = ?", expiresAt, userID)
	return err
}

func SetUserPremiumDuration(userID int64, durationDays int) error {
	expiresAt := time.Now().UTC().AddDate(0, 0, durationDays).Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE users SET is_premium = TRUE, premium_expires_at = ? WHERE id = ?", expiresAt, userID)
	return err
}

func DeactivateAlert(alertID int64) error {
	_, err := db.Exec(
		"UPDATE alerts SET active = FALSE, triggered_at = CURRENT_TIMESTAMP WHERE id = ?",
		alertID,
	)
	return err
}

func GetSettings(userID int64) (Settings, error) {
	if _, err := db.Exec(
		"INSERT OR IGNORE INTO settings (user_id) VALUES (?)",
		userID,
	); err != nil {
		return Settings{}, err
	}
	var s Settings
	s.UserID = userID
	err := db.QueryRow(
		"SELECT daily_digest, digest_time, timezone FROM settings WHERE user_id = ?",
		userID,
	).Scan(&s.DailyDigest, &s.DigestTime, &s.Timezone)
	return s, err
}

func SetDigest(userID int64, on bool) error {
	if _, err := db.Exec(
		"INSERT OR IGNORE INTO settings (user_id) VALUES (?)",
		userID,
	); err != nil {
		return err
	}
	_, err := db.Exec(
		"UPDATE settings SET daily_digest = ? WHERE user_id = ?",
		on, userID,
	)
	return err
}

func GetTriggeredAlerts(userID int64, limit int) ([]Alert, error) {
	rows, err := db.Query(
		"SELECT "+alertColumns+" FROM alerts WHERE user_id = ? AND triggered_at IS NOT NULL ORDER BY triggered_at DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func SetDigestTime(userID int64, digestTime string) error {
	if _, err := db.Exec(
		"INSERT OR IGNORE INTO settings (user_id) VALUES (?)",
		userID,
	); err != nil {
		return err
	}
	_, err := db.Exec(
		"UPDATE settings SET digest_time = ? WHERE user_id = ?",
		digestTime, userID,
	)
	return err
}

func GetDigestSubscribers(digestTime string) ([]int64, error) {
	rows, err := db.Query(
		"SELECT user_id FROM settings WHERE daily_digest = TRUE AND digest_time = ?",
		digestTime,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
