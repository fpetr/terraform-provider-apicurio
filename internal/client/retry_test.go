package client

import "testing"

func TestIsRetryableDeadlock(t *testing.T) {
	body := `{"message":"com.mysql.cj.jdbc.exceptions.MySQLTransactionRollbackException: Deadlock found when trying to get lock; try restarting transaction","error_code":500,"detail":"MySQLTransactionRollbackException: Deadlock found when trying to get lock; try restarting transaction","name":"RuntimeSqlException"}`
	if !isRetryableDeadlock(500, body) {
		t.Fatalf("expected deadlock to be retryable")
	}
	pgDeadlock := `{"message":"org.postgresql.util.PSQLException: ERROR: deadlock detected","detail":"Process 123 waits for ShareLock on transaction 456; blocked by process 789."}`
	if !isRetryableDeadlock(500, pgDeadlock) {
		t.Fatalf("expected postgres deadlock to be retryable")
	}
	pgSer := `{"message":"ERROR: could not serialize access due to read/write dependencies among transactions","detail":"SQLSTATE 40001"}`
	if !isRetryableDeadlock(500, pgSer) {
		t.Fatalf("expected postgres serialization failure to be retryable")
	}
	if isRetryableDeadlock(500, `{"message":"some other server error"}`) {
		t.Fatalf("expected non-deadlock 500 to NOT be retryable")
	}
	if isRetryableDeadlock(409, body) {
		t.Fatalf("expected non-5xx to NOT be retryable")
	}
}
