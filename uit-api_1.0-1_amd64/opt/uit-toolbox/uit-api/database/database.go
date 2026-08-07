package database

import (
	"context"
	"crypto"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"uit-api/auth"
	"uit-api/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var (
	stdLibDBPool atomic.Pointer[sql.DB]
	pgxPool      atomic.Pointer[pgxpool.Pool]
)

func InitDatabasePools() (*sql.DB, *pgxpool.Pool, error) {
	// Get DB credentials
	dbConnectionInfo, err := GetDatabaseCredentials()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get database credentials: %w", err)
	}

	// Create DB connection
	dbConn, err := NewDBConnection(dbConnectionInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	pg, err := NewPGXPool(dbConnectionInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	stdLibDBPool.Store(dbConn)

	pgxPool.Store(pg)

	// Create admin user
	if err = CreateAdminUser(); err != nil {
		return nil, nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	return stdLibDBPool.Load(), pgxPool.Load(), nil
}

func VerifyRowsAffected(result sql.Result, expectedRowCount int64) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected != expectedRowCount {
		return fmt.Errorf("%w: expected exactly %d row(s), got %d", types.DatabaseAffectedRowsError, expectedRowCount, rowsAffected)
	}
	return nil
}

// Go value types to SQL NULL types //

// Go bool to SQL NULL bool, where false is treated as NULL
func boolToSqlNull(b bool) sql.NullBool {
	if !b {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: b, Valid: true}
}

// Go float64 to SQL NULL float64, where 0.0 is treated as NULL
func float64ToSqlNull(f float64) sql.NullFloat64 {
	if f == 0 {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}

// Go int to SQL NULL int64, where 0 is treated as NULL
func intToSqlNull(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(i), Valid: true}
}

// Go int64 to SQL NULL int64, where 0 is treated as NULL
func int64ToSqlNull(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: i, Valid: true}
}

// Go string to SQL NULL string, where empty string is treated as NULL
func stringToSqlNull(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// Go time.Duration to SQL NULL int64, where 0 is treated as NULL
func durationToSqlNull(d time.Duration) sql.NullInt64 {
	if d == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(d), Valid: true}
}

// Go time.Time to SQL NULL time.Time, where time.IsZero() is treated as NULL
func timeToSqlNull(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// Go uuid.UUID to SQL NULL string, where empty UUID is treated as NULL
func uuidToSqlNull(u uuid.UUID) sql.NullString {
	if u.String() == "" || u == (uuid.UUID{}) || u == uuid.Nil {
		return sql.NullString{}
	}
	return sql.NullString{String: u.String(), Valid: true}
}

// SQL NULL types to Go value types //

// SQL NULL bool to Go bool, where NULL is treated as false
func sqlNullToBool(v sql.NullBool) bool {
	if v.Valid {
		return v.Bool
	}
	return false
}

// SQL NULL int64 to Go int, where NULL is treated as 0
func sqlNullToInt(v sql.NullInt64) int {
	if v.Valid {
		return int(v.Int64)
	}
	return 0
}

// SQL NULL int64 to Go int64, where NULL is treated as 0
func sqlNullToInt64(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

// SQL NULL float64 to Go float64, where NULL is treated as 0.0
func sqlNullToFloat64(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0.0
}

// SQL NULL string to Go string, where NULL is treated as empty string
func sqlNullToString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// SQL NULL int64 to Go time.Duration, where NULL is treated as 0
func sqlNullToDuration(v sql.NullInt64) time.Duration {
	if v.Valid {
		return time.Duration(v.Int64)
	}
	return 0
}

// SQL NULL time to Go time.Time, where NULL is treated as zero time
func sqlNullToTime(v sql.NullTime) time.Time {
	if v.Valid {
		return v.Time
	}
	return time.Time{}
}

func sqlNullToUUID(v sql.NullString) uuid.UUID {
	if v.Valid {
		u, err := uuid.Parse(v.String)
		if err != nil {
			return uuid.Nil
		}
		return u
	}
	return uuid.Nil
}

// Go pointer types to SQL NULL types //

// Go *bool to SQL NULL bool
func boolPtrToSqlNull(p *bool) sql.NullBool {
	if p == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *p, Valid: true}
}

// Go *float64 to SQL NULL float64
func float64PtrToSqlNull(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

// Go *int to SQL NULL int64
func intPtrToSqlNull(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// Go *int64 to SQL NULL int64
func int64PtrToSqlNull(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// Go *string to SQL NULL string
func stringPtrToSqlNull(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	} else if *p == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// Go *time.Duration to SQL NULL int64
func durationPtrToSqlNull(p *time.Duration) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// Go *time.Time to SQL NULL time
func timePtrToSqlNull(p *time.Time) sql.NullTime {
	if p == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *p, Valid: true}
}

// SQL NULL types to Go pointer types //

// SQL NULL bool to Go *bool
func sqlBoolToPtr(v sql.NullBool) *bool {
	if v.Valid {
		return &v.Bool
	}
	return nil
}

// SQL NULL int64 to Go *time.Duration
func sqlDurationToPtr(v sql.NullInt64) *time.Duration {
	if v.Valid {
		d := time.Duration(v.Int64)
		return &d
	}
	return nil
}

// SQL NULL float64 to Go *float64
func sqlFloat64ToPtr(v sql.NullFloat64) *float64 {
	if v.Valid {
		return &v.Float64
	}
	return nil
}

// SQL NULL int64 to Go *int64
func sqlInt64ToPtr(v sql.NullInt64) *int64 {
	if v.Valid {
		return &v.Int64
	}
	return nil
}

// SQL NULL string to Go *string
func sqlStringToPtr(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}

// SQL NULL time to Go *time.Time
func sqlTimeToPtr(v sql.NullTime) *time.Time {
	if v.Valid {
		return &v.Time
	}
	return nil
}

// CSV and PGSQL array type conversion functions //

// Go []string to concatenated for PGSQL array representation
func ptrSliceToString(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	return strings.Join(slice, ", ")
}

// Go *bool to string, where nil is treated as empty string
func ptrBoolToString(p *bool) string {
	if p == nil {
		return ""
	}
	return strconv.FormatBool(*p)
}

// Go *int64 to string, where nil is treated as empty string
func ptrInt64ToString(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

// Go *string to string, where nil is treated as empty string
func ptrStringToString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Go *time.Time to string, where nil is treated as empty string
func ptrTimeToString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func CreateAdminUser() error {
	db, err := GetDatabasePool()
	if err != nil {
		return fmt.Errorf("error getting database connection in CreateAdminUser: %w", err)
	}
	adminUsername, adminPasswd, err := auth.GetAdminCredentials()
	if err != nil {
		return fmt.Errorf("failed to get admin credentials: %w", err)
	}

	if strings.TrimSpace(adminUsername) == "" {
		return fmt.Errorf("admin username is empty")
	}
	usernameHash := crypto.SHA256.New()
	usernameHash.Write([]byte(adminUsername))
	adminUsernameHash := hex.EncodeToString(usernameHash.Sum(nil))

	if strings.TrimSpace(adminPasswd) == "" {
		return fmt.Errorf("admin password is empty")
	}
	passwordHash := crypto.SHA256.New()
	passwordHash.Write([]byte(adminPasswd))
	adminPasswdHash := hex.EncodeToString(passwordHash.Sum(nil))

	bcryptHashBytes, err := bcrypt.GenerateFromPassword([]byte(adminPasswdHash), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash admin password: %w", err)
	}
	bcryptHashString := string(bcryptHashBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Delete and recreate admin user in table logins
	sqlCode := `
    INSERT INTO logins (username, password, common_name, is_admin, enabled)
    VALUES ($1, $2, 'admin', TRUE, TRUE)
    ON CONFLICT (username)
    DO UPDATE SET 
		username = EXCLUDED.username,
		common_name = EXCLUDED.common_name,
		is_admin = EXCLUDED.is_admin,
		enabled = EXCLUDED.enabled,
		password = EXCLUDED.password;
    `

	_, err = db.ExecContext(ctx, sqlCode, adminUsernameHash, bcryptHashString)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	return nil
}
