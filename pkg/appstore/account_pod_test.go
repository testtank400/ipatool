package appstore

import (
	gohttp "net/http"
	"net/url"
	"testing"
)

type stubCookieJar struct {
	cookies map[string][]*gohttp.Cookie
}

func (s *stubCookieJar) Cookies(u *url.URL) []*gohttp.Cookie {
	if s.cookies == nil {
		return nil
	}
	return s.cookies[u.Host]
}

func (s *stubCookieJar) SetCookies(u *url.URL, cookies []*gohttp.Cookie) {
	if s.cookies == nil {
		s.cookies = map[string][]*gohttp.Cookie{}
	}
	s.cookies[u.Host] = cookies
}

func (s *stubCookieJar) Save() error { return nil }

func TestEnsureAccountPodFromCookie(t *testing.T) {
	jar := &stubCookieJar{
		cookies: map[string][]*gohttp.Cookie{
			"buy.itunes.apple.com": {{Name: "itspod", Value: "26"}},
		},
	}
	as := &appstore{cookieJar: jar}

	acc := as.ensureAccountPod(Account{})
	if acc.Pod != "26" {
		t.Fatalf("expected pod 26, got %q", acc.Pod)
	}

	// Existing pod wins
	acc = as.ensureAccountPod(Account{Pod: "7"})
	if acc.Pod != "7" {
		t.Fatalf("expected existing pod 7, got %q", acc.Pod)
	}
}

func TestEnsureAccountPodNilJar(t *testing.T) {
	as := &appstore{}
	acc := as.ensureAccountPod(Account{})
	if acc.Pod != "" {
		t.Fatalf("expected empty pod, got %q", acc.Pod)
	}
}
