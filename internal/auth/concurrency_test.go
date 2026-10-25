package auth_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/config"
)

// burst fires n requests at once and counts the status codes; ipFor picks each request's client address.
func burst(e *env, n int, method, path string, body any, cookie string, ipFor func(i int) string) map[int]int {
	e.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		e.t.Fatal(err)
	}
	var (
		mu     sync.Mutex
		counts = map[int]int{}
		wg     sync.WaitGroup
		start  = make(chan struct{})
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
			r.RemoteAddr = ipFor(i) + ":5000"
			r.Header.Set("Content-Type", "application/json")
			if cookie != "" {
				r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
			}
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, r)
			mu.Lock()
			counts[rec.Code]++
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	return counts
}

func sameIP(string) func(int) string { return func(int) string { return "192.0.2.10" } }

// expectBudget fails unless exactly one lock window's worth of tries was evaluated and the rest refused.
func expectBudget(t *testing.T, counts map[int]int, n, evaluatedStatus int) {
	t.Helper()
	if got := counts[evaluatedStatus]; got != 5 {
		t.Errorf("%d tries were evaluated (status %d), want exactly 5; all: %v", got, evaluatedStatus, counts)
	}
	if got := counts[http.StatusTooManyRequests]; got != n-5 {
		t.Errorf("%d tries were refused, want %d; all: %v", got, n-5, counts)
	}
}

func TestConcurrentPasswordGuessesGetOneLockWindowsBudget(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	counts := burst(e, 200, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "alice", "password": "wrong password here"}, "", sameIP(""))
	expectBudget(t, counts, 200, http.StatusUnauthorized)
}

func TestConcurrentSecondStepGuessesGetOneLockWindowsBudget(t *testing.T) {
	e, _, _, _ := signedInWithTOTP(t)
	c := loginStep1(t, e)
	counts := burst(e, 500, http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "000000"}, c.cookie, sameIP(""))
	expectBudget(t, counts, 500, http.StatusUnauthorized)
}

func TestConcurrentGuessesFromManyAddressesAreStillBoundByTheUsername(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	counts := burst(e, 100, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "alice", "password": "wrong password here"}, "",
		func(i int) string { return fmt.Sprintf("198.51.100.%d", i+1) })
	expectBudget(t, counts, 100, http.StatusUnauthorized)
	e.client().login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
}

func TestConcurrentGuessesFromOneAddressAtManyUsernamesAreBoundByTheAddress(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	var mu sync.Mutex
	counts := map[int]int{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c := e.client()
			code := c.login(fmt.Sprintf("victim%d", i), "wrong password here").Code
			mu.Lock()
			counts[code]++
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	expectBudget(t, counts, 100, http.StatusUnauthorized)
}

func TestConcurrentCurrentPasswordGuessesGetOneLockWindowsBudget(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	counts := burst(e, 200, http.MethodPost, "/api/v1/me/password", map[string]string{"current_password": "not my password!!", "new_password": newPassword}, c.cookie, sameIP(""))
	expectBudget(t, counts, 200, http.StatusUnprocessableEntity)
	// The password did not change.
	e.now = e.now.Add(time.Hour)
	e.client().mustLogin("alice", goodPassword)
}

func TestConcurrentCodeGuessesWhenRemovingTOTPGetOneLockWindowsBudget(t *testing.T) {
	e := newEnv(t, config.Config{AuthMode: config.AuthModeDisabled})
	e.addUser("root", "", "admin")
	c := e.client()
	c.enableTOTP()
	counts := burst(e, 300, http.MethodDelete, "/api/v1/me/totp", map[string]string{"code": "000000"}, "", sameIP(""))
	expectBudget(t, counts, 300, http.StatusUnprocessableEntity)
}

func TestSuccessfulSignInsDoNotUseUpTheAddressBudget(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	// Many more sign-ins than the failure budget, one after another from the same address.
	for range 30 {
		c.mustLogin("alice", goodPassword)
	}
}

func TestSignInsThatStopAtTheSecondStepDoNotUseUpTheAddressBudget(t *testing.T) {
	e, _, _, _ := signedInWithTOTP(t)
	for range 30 {
		r := e.client().login("alice", goodPassword)
		if r.Code != http.StatusOK {
			t.Fatalf("password step refused: %d %s", r.Code, r.Body)
		}
	}
}
