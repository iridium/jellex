package plex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iridium/jellex/assets"
	"github.com/iridium/jellex/internal/auth"
)

// Browser sign-in: Plex Web runs without a plex.tv account (patched), so
// jellex asks for a Jellyfin login before serving it and acts as that user
// on every request carrying the session cookie.

type ctxKey int

const sessionKey ctxKey = iota

// sessionFrom returns the signed-in session on a request context.
func sessionFrom(ctx context.Context) (auth.Session, bool) {
	ses, ok := ctx.Value(sessionKey).(auth.Session)
	return ses, ok
}

func (s *Server) loginRoutes() {
	m := s.mux
	m.HandleFunc("GET /web/login", s.handleLoginPage)
	m.HandleFunc("POST /web/login", s.handlePasswordLogin)
	m.HandleFunc("POST /web/login/quickconnect", s.handleQuickConnectStart)
	m.HandleFunc("GET /web/login/quickconnect/{id}", s.handleQuickConnectPoll)
	m.HandleFunc("GET /web/logout", s.handleLogout)
}

// authRequired reports whether a request needs a session. The Plex Web
// client's static files are public (they hold no data); the page that boots
// it, and the whole API, are not.
func authRequired(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodOptions:
		return false
	case p == "/identity", p == "/favicon.ico":
		return false
	case strings.HasPrefix(p, "/web/login"), p == "/web/logout", strings.HasPrefix(p, "/web/jellex/"):
		return false
	case p == "/web", p == "/web/", p == "/web/index.html":
		return true
	case strings.HasPrefix(p, "/web/"):
		return false
	}
	return true
}

// authenticate attaches the request's session to its context, or answers it
// (redirect to the login page, or 401) and returns nil.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) *http.Request {
	if s.logins == nil {
		return r
	}
	_, ses, ok := s.logins.FromRequest(r)
	if ok {
		return r.WithContext(context.WithValue(r.Context(), sessionKey, ses))
	}
	if !authRequired(r) {
		return r
	}
	if strings.HasPrefix(r.URL.Path, "/web") {
		http.Redirect(w, r, "/web/login", http.StatusFound)
		return nil
	}
	http.Error(w, "sign in at /web/login", http.StatusUnauthorized)
	return nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, id string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, ses auth.Session) error {
	id, err := s.logins.Start(ses)
	if err != nil {
		return err
	}
	s.setSessionCookie(w, r, id, int((30 * 24 * time.Hour).Seconds()))
	slog.Info("signed in", "user", ses.UserName)
	return nil
}

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign in · jellex</title>
<link rel="icon" href="/favicon.ico">
<style>
  :root { color-scheme: dark; --bg: #16171d; --panel: #1f2028; --line: #2e2f3a; --text: #ececf1; --muted: #9a9bab; --accent: #00a4dc; --accent2: #aa5cc3; --err: #ff6b6b; }
  * { box-sizing: border-box; }
  body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: var(--bg); color: var(--text); font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif; padding: 16px; }
  main { width: 100%; max-width: 360px; background: var(--panel); border: 1px solid var(--line); border-radius: 14px; padding: 32px 28px; }
  .logo { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; }
  .logo svg { width: 36px; height: 36px; }
  .logo span { font-size: 22px; font-weight: 650; letter-spacing: .2px; }
  p.sub { color: var(--muted); margin: 0 0 22px; font-size: 14px; }
  label { display: block; font-size: 13px; color: var(--muted); margin: 14px 0 6px; }
  input { width: 100%; padding: 10px 12px; border-radius: 8px; border: 1px solid var(--line); background: var(--bg); color: var(--text); font: inherit; }
  input:focus { outline: 2px solid var(--accent); outline-offset: -1px; border-color: transparent; }
  button { width: 100%; margin-top: 20px; padding: 11px; border: 0; border-radius: 8px; font: inherit; font-weight: 600; cursor: pointer; }
  .primary { background: linear-gradient(135deg, var(--accent2), var(--accent)); color: #fff; }
  .secondary { background: transparent; color: var(--text); border: 1px solid var(--line); margin-top: 10px; }
  .err { color: var(--err); font-size: 14px; margin: 14px 0 0; }
  .divider { display: flex; align-items: center; gap: 10px; color: var(--muted); font-size: 12px; margin: 22px 0 0; }
  .divider::before, .divider::after { content: ""; flex: 1; border-top: 1px solid var(--line); }
  #qc { display: none; text-align: center; margin-top: 16px; }
  #qc .code { font: 600 30px/1 ui-monospace, monospace; letter-spacing: 6px; margin: 10px 0; }
  #qc p { color: var(--muted); font-size: 13px; margin: 0; }
</style>
</head>
<body>
<main>
  <div class="logo">{{.Logo}}<span>jellex</span></div>
  <p class="sub">Sign in with your Jellyfin account.</p>
  <form method="post" action="/web/login">
    <label for="u">Username</label>
    <input id="u" name="username" autocomplete="username" required autofocus value="{{.Username}}">
    <label for="p">Password</label>
    <input id="p" name="password" type="password" autocomplete="current-password">
    {{if .Error}}<p class="err" role="alert">{{.Error}}</p>{{end}}
    <button class="primary" type="submit">Sign in</button>
  </form>
  <div class="divider">or</div>
  <button class="secondary" id="qcbtn" type="button">Use Quick Connect</button>
  <div id="qc" aria-live="polite">
    <p>Enter this code in a Jellyfin app you're signed in to (Settings → Quick Connect):</p>
    <div class="code" id="qccode"></div>
    <p id="qcstatus">Waiting for approval…</p>
  </div>
</main>
<script>
document.getElementById('qcbtn').addEventListener('click', async () => {
  const status = document.getElementById('qcstatus');
  document.getElementById('qc').style.display = 'block';
  const res = await fetch('/web/login/quickconnect', { method: 'POST' });
  const body = await res.json();
  if (!res.ok) { status.textContent = body.error || 'Quick Connect failed.'; return; }
  document.getElementById('qccode').textContent = body.code;
  const poll = async () => {
    const r = await fetch('/web/login/quickconnect/' + body.id);
    const s = await r.json();
    if (s.done) { location.href = '/web/'; return; }
    if (!r.ok) { status.textContent = s.error || 'Quick Connect failed.'; return; }
    setTimeout(poll, 2000);
  };
  setTimeout(poll, 2000);
});
</script>
</body>
</html>
`))

func (s *Server) renderLogin(w http.ResponseWriter, status int, username, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	loginPage.Execute(w, map[string]any{
		"Logo":     template.HTML(assets.Logo),
		"Username": username,
		"Error":    errMsg,
	})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := sessionFrom(r.Context()); ok {
		http.Redirect(w, r, "/web/", http.StatusFound)
		return
	}
	s.renderLogin(w, http.StatusOK, "", "")
}

func (s *Server) handlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSpace(r.FormValue("username"))
	ses, err := s.logins.Password(r.Context(), user, r.FormValue("password"))
	if errors.Is(err, auth.ErrBadCredentials) {
		s.renderLogin(w, http.StatusUnauthorized, user, "Wrong username or password.")
		return
	}
	if err != nil {
		slog.Error("jellyfin sign-in failed", "err", err)
		s.renderLogin(w, http.StatusBadGateway, user, "Couldn't reach Jellyfin. Try again.")
		return
	}
	if err := s.startSession(w, r, ses); err != nil {
		fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/web/", http.StatusSeeOther)
}

// pendingQuickConnect tracks Quick Connect sign-ins by a random ID, so the
// Jellyfin secret never reaches the browser.
type pendingQuickConnect struct {
	mu sync.Mutex
	by map[string]pendingQC
}

type pendingQC struct {
	qc      auth.QuickConnect
	expires time.Time
}

func (p *pendingQuickConnect) add(qc auth.QuickConnect) string {
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.by == nil {
		p.by = map[string]pendingQC{}
	}
	for k, v := range p.by {
		if time.Now().After(v.expires) {
			delete(p.by, k)
		}
	}
	p.by[id] = pendingQC{qc, time.Now().Add(10 * time.Minute)}
	return id
}

func (p *pendingQuickConnect) get(id string) (auth.QuickConnect, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.by[id]
	if !ok || time.Now().After(v.expires) {
		return auth.QuickConnect{}, false
	}
	return v.qc, true
}

func (p *pendingQuickConnect) remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.by, id)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) handleQuickConnectStart(w http.ResponseWriter, r *http.Request) {
	qc, err := s.logins.StartQuickConnect(r.Context())
	if errors.Is(err, auth.ErrQuickConnectDisabled) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Quick Connect is turned off on this Jellyfin server."})
		return
	}
	if err != nil {
		slog.Error("quick connect", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Couldn't reach Jellyfin."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": s.quickConnects.add(qc), "code": qc.Code})
}

func (s *Server) handleQuickConnectPoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	qc, ok := s.quickConnects.get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "This code expired. Start again."})
		return
	}
	ses, done, err := s.logins.PollQuickConnect(r.Context(), qc)
	if err != nil {
		slog.Error("quick connect poll", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Couldn't reach Jellyfin."})
		return
	}
	if !done {
		writeJSON(w, http.StatusOK, map[string]bool{"done": false})
		return
	}
	s.quickConnects.remove(id)
	if err := s.startSession(w, r, ses); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"done": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.logins != nil {
		if id, _, ok := s.logins.FromRequest(r); ok {
			s.logins.End(r.Context(), id)
		}
		s.setSessionCookie(w, r, "", -1)
	}
	http.Redirect(w, r, "/web/login", http.StatusFound)
}
