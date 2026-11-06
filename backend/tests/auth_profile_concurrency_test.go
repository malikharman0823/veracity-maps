package tests

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/db"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	mssql "github.com/microsoft/go-mssqldb"
)

// All interleaving controls live in tests. SQL statements run against the real
// driver and database; wrappers expose the boundary before a selected statement.
type statementGate struct {
	ready, release    chan struct{}
	once, releaseOnce sync.Once
}

func newStatementGate(t *testing.T) *statementGate {
	g := &statementGate{ready: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(g.open)
	return g
}
func (g *statementGate) open() { g.releaseOnce.Do(func() { close(g.release) }) }
func (g *statementGate) wait(ctx context.Context) error {
	var err error
	g.once.Do(func() {
		close(g.ready)
		select {
		case <-g.release:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}
func (g *statementGate) await(t *testing.T) {
	t.Helper()
	select {
	case <-g.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("operation did not reach its controlled boundary")
	}
}

type gatedSQLConnector struct {
	driver.Connector
	prefix string
	before func(context.Context, driver.Conn) error
	once   sync.Once
}

func (c *gatedSQLConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &gatedSQLConn{Conn: conn, owner: c}, nil
}

type gatedSQLConn struct {
	driver.Conn
	owner *gatedSQLConnector
}

func (c *gatedSQLConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
}
func (c *gatedSQLConn) CheckNamedValue(v *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(v)
}
func (c *gatedSQLConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	stmt, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &gatedSQLStmt{Stmt: stmt, owner: c.owner, conn: c.Conn, query: query}, nil
}

type gatedSQLStmt struct {
	driver.Stmt
	owner *gatedSQLConnector
	query string
	conn  driver.Conn
}

func (s *gatedSQLStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.owner.before != nil && strings.HasPrefix(strings.TrimSpace(s.query), s.owner.prefix) {
		var err error
		s.owner.once.Do(func() { err = s.owner.before(ctx, s.conn) })
		if err != nil {
			return nil, err
		}
	}
	return s.Stmt.(driver.StmtExecContext).ExecContext(ctx, args)
}
func (s *gatedSQLStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.Stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
}

func controlledAuthDatabase(t *testing.T, prefix string) (*sql.DB, *gatedSQLConnector) {
	t.Helper()
	if os.Getenv("MSSQL_TEST_CONNECTION_STRING") == "" {
		t.Skip("MSSQL_TEST_CONNECTION_STRING is not set; a dedicated SQL Server test database is required")
	}
	connector, err := mssql.NewConnector(os.Getenv("MSSQL_TEST_CONNECTION_STRING"))
	if err != nil {
		t.Fatal("invalid test SQL connector configuration")
	}
	control := &gatedSQLConnector{Connector: connector, prefix: prefix}
	database := sql.OpenDB(control)
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("controlled SQL connection unavailable (details withheld; type %T)", err)
	}
	if err := db.RunMigrations(ctx, database, "../migrations"); err != nil {
		t.Fatal(err)
	}
	return database, control
}

func driverQuery(ctx context.Context, conn driver.Conn, query string) ([]driver.Value, error) {
	stmt, err := conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.(driver.StmtQueryContext).QueryContext(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]driver.Value, len(rows.Columns()))
	if err := rows.Next(values); err != nil {
		return nil, err
	}
	return values, nil
}

func controlledAuthUser(t *testing.T, database *sql.DB) (*repository.MSSQLAuthRepository, *models.User, string) {
	t.Helper()
	r := repository.NewMSSQLAuthRepository(database)
	id, err := models.GenerateRandomHex(8)
	if err != nil {
		t.Fatal(err)
	}
	u, err := r.Register(context.Background(), "Interleaving", id+"@concurrency.example.com", "Current123!")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec("DELETE FROM Users WHERE id = @id", sql.Named("id", u.ID)) })
	token, err := r.CreateToken(context.Background(), u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return r, u, token.Token
}

// On the split Login implementation, pause after successful verification. On
// atomic Login, pause before the indivisible operation. A password change must
// therefore either revoke its issued token or make the old credentials fail.
type gatedLoginRepository struct {
	repository.AuthRepo
	gate *statementGate
}

func (r *gatedLoginRepository) Authenticate(ctx context.Context, email, password string) (*models.User, error) {
	u, err := r.AuthRepo.Authenticate(ctx, email, password)
	if err != nil {
		return nil, err
	}
	if err := r.gate.wait(ctx); err != nil {
		return nil, err
	}
	return u, nil
}
func (r *gatedLoginRepository) AuthenticateAndCreateToken(ctx context.Context, email, password string, duration time.Duration) (*models.User, *models.AuthToken, error) {
	if err := r.gate.wait(ctx); err != nil {
		return nil, nil, err
	}
	return r.AuthRepo.AuthenticateAndCreateToken(ctx, email, password, duration)
}

func TestAuthenticationLoginRejectsPasswordChangedBeforeAtomicOperation(t *testing.T) {
	_, database, authRepo, user, currentToken, _, _ := profileHarness(t)
	gate := newStatementGate(t)
	handler := auth.NewHandler(&gatedLoginRepository{AuthRepo: authRepo, gate: gate})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"`+user.Email+`","password":"Current123!"}`)).WithContext(ctx)
		res := httptest.NewRecorder()
		handler.Login(res, req)
		done <- res
	}()
	gate.await(t)
	if err := repository.NewMSSQLProfileRepository(database).ChangePassword(ctx, user.ID, "Current123!", "Replacement123!", currentToken); err != nil {
		t.Fatal(err)
	}
	gate.open()
	res := <-done
	if res.Code != 401 {
		t.Fatalf("stale old-password login returned %d; it must not issue a token", res.Code)
	}
	var live int
	if err := database.QueryRow("SELECT COUNT(*) FROM AuthTokens WHERE user_id = @id AND revoked_at IS NULL", sql.Named("id", user.ID)).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 1 {
		t.Fatalf("password change left %d live tokens, expected only the current token", live)
	}
}

func TestAuthenticationAtomicLoginCommitsBeforePasswordChange(t *testing.T) {
	database, control := controlledAuthDatabase(t, "INSERT INTO AuthTokens")
	loginRepo, user, currentToken := controlledAuthUser(t, database)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var transactionCount, userLocks int64
	control.before = func(ctx context.Context, conn driver.Conn) error {
		// Observe actual lock state before inserting the bearer token. This uses
		// the held SQL session, not an extra connection or timing assumptions.
		values, err := driverQuery(ctx, conn, `SELECT @@TRANCOUNT, (SELECT COUNT(*) FROM sys.dm_tran_locks l
		INNER JOIN sys.partitions p ON p.hobt_id = l.resource_associated_entity_id
		WHERE p.object_id = OBJECT_ID('Users') AND l.request_session_id = @@SPID
		AND l.request_status = 'GRANT' AND l.request_mode IN ('U','X','RangeS-U'))`)
		if err != nil {
			return err
		}
		transactionCount, userLocks = values[0].(int64), values[1].(int64)
		return nil
	}
	handler := auth.NewHandler(loginRepo)
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"`+user.Email+`","password":"Current123!"}`)).WithContext(ctx)
	res := httptest.NewRecorder()
	handler.Login(res, req)
	if res.Code != 200 {
		t.Fatalf("atomic login failed: %d %s", res.Code, res.Body)
	}
	if transactionCount != 1 || userLocks == 0 {
		t.Fatalf("login released credential protection before token insertion: transactions=%d user locks=%d", transactionCount, userLocks)
	}
	var response struct {
		Data models.AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewMSSQLProfileRepository(database).ChangePassword(ctx, user.ID, "Current123!", "Replacement123!", currentToken); err != nil {
		t.Fatal(err)
	}
	if _, err := loginRepo.ValidateToken(ctx, response.Data.Token); !errors.Is(err, repository.ErrInvalidToken) {
		t.Fatal("password change did not revoke the login that committed first")
	}
}

func TestAuthenticationStaleLegacyUpgradeCannotRestoreOldPassword(t *testing.T) {
	database, control := controlledAuthDatabase(t, "UPDATE Users SET password_hash")
	authRepo, user, _ := controlledAuthUser(t, database)
	const legacyHash = "d5cd010d12c98482b66f2aff25435764da31fe652d4f0698f0ec9df58c0d97a6"
	if _, err := database.Exec("UPDATE Users SET password_hash = @hash, salt = 'legacy-salt' WHERE id = @id", sql.Named("hash", legacyHash), sql.Named("id", user.ID)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	newHash, err := models.HashPassword("Replacement123!")
	if err != nil {
		t.Fatal(err)
	}
	control.before = func(ctx context.Context, conn driver.Conn) error {
		// Commit the replacement after the old credentials were read and before
		// the legacy upgrade executes. A second transport session is unnecessary
		// for this exact stale-read interleaving.
		stmt, err := conn.(driver.ConnPrepareContext).PrepareContext(ctx, "UPDATE Users SET password_hash = @replacement, salt = '' WHERE id = @user")
		if err != nil {
			return err
		}
		defer stmt.Close()
		_, err = stmt.(driver.StmtExecContext).ExecContext(ctx, []driver.NamedValue{{Name: "replacement", Ordinal: 1, Value: newHash}, {Name: "user", Ordinal: 2, Value: user.ID}})
		return err
	}
	if _, err := authRepo.Authenticate(ctx, user.Email, "Password123!"); !errors.Is(err, repository.ErrInvalidPassword) {
		t.Fatalf("stale legacy authentication was not rejected: %v", err)
	}
	if _, err := authRepo.Authenticate(ctx, user.Email, "Replacement123!"); err != nil {
		t.Fatal("stale legacy upgrade overwrote the new password")
	}
	if _, err := authRepo.Authenticate(ctx, user.Email, "Password123!"); !errors.Is(err, repository.ErrInvalidPassword) {
		t.Fatal("old legacy password was restored")
	}
}

func TestAuthenticationFilesystemAtomicLoginPersistsHashedToken(t *testing.T) {
	root := t.TempDir()
	r, err := repository.NewAuthRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	user, err := r.Register(context.Background(), "Atomic", "atomic@example.com", "Current123!")
	if err != nil {
		t.Fatal(err)
	}
	u, token, err := r.AuthenticateAndCreateToken(context.Background(), user.Email, "Current123!", time.Hour)
	if err != nil || u.ID != user.ID {
		t.Fatalf("atomic filesystem login: %v", err)
	}
	restarted, err := repository.NewAuthRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ValidateToken(context.Background(), token.Token); err != nil {
		t.Fatal("atomic login token was not persisted")
	}
	if _, _, err := r.AuthenticateAndCreateToken(context.Background(), user.Email, "wrong", time.Hour); !errors.Is(err, repository.ErrInvalidPassword) {
		t.Fatal("wrong password issued a token")
	}
}
