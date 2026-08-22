package main

import (
	"errors"
	"testing"
)

type fakeLanguageService struct {
	running     bool
	startCalled bool
	startErr    error
	port        int
}

func (f *fakeLanguageService) IsLibreTranslateRunning() bool {
	return f.running
}

func (f *fakeLanguageService) StartLibreTranslate() error {
	f.startCalled = true
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakeLanguageService) LibreTranslatePort() int {
	return f.port
}

type fakeLanguageLister struct {
	pairs []LanguagePair
	err   error
}

func (f fakeLanguageLister) ListLanguages() ([]LanguagePair, error) {
	return f.pairs, f.err
}

func TestListAvailableLanguagesStartsLibreTranslateWhenIdle(t *testing.T) {
	svc := &fakeLanguageService{port: 5000}

	prevFactory := newLanguageLister
	t.Cleanup(func() {
		newLanguageLister = prevFactory
	})
	newLanguageLister = func(port int) languageLister {
		if port != 5000 {
			t.Fatalf("newLanguageLister() port = %d, want 5000", port)
		}
		return fakeLanguageLister{
			pairs: []LanguagePair{{Source: "en", Target: "ja"}},
		}
	}

	langs, err := listAvailableLanguages(svc)
	if err != nil {
		t.Fatalf("listAvailableLanguages() error = %v", err)
	}
	if !svc.startCalled {
		t.Fatal("listAvailableLanguages() did not start LibreTranslate")
	}
	if len(langs) != 1 || langs[0] != (LanguagePair{Source: "en", Target: "ja"}) {
		t.Fatalf("listAvailableLanguages() = %#v, want the fake pair", langs)
	}
}

func TestListAvailableLanguagesReturnsEmptyWhenLibreTranslateIsUnavailable(t *testing.T) {
	svc := &fakeLanguageService{
		port:     5000,
		startErr: errors.New(`libretranslate executable "libretranslate" not found in PATH`),
	}

	prevFactory := newLanguageLister
	t.Cleanup(func() {
		newLanguageLister = prevFactory
	})

	listerCalled := false
	newLanguageLister = func(port int) languageLister {
		listerCalled = true
		return fakeLanguageLister{
			pairs: []LanguagePair{{Source: "en", Target: "ja"}},
		}
	}

	langs, err := listAvailableLanguages(svc)
	if err != nil {
		t.Fatalf("listAvailableLanguages() error = %v, want nil", err)
	}
	if !svc.startCalled {
		t.Fatal("listAvailableLanguages() did not attempt to start LibreTranslate")
	}
	if listerCalled {
		t.Fatal("listAvailableLanguages() should not query languages when LibreTranslate is unavailable")
	}
	if len(langs) != 0 {
		t.Fatalf("listAvailableLanguages() = %#v, want an empty list", langs)
	}
}

func TestLocalServiceBaseURLUsesIPv4Loopback(t *testing.T) {
	if got := localServiceBaseURL(5000); got != "http://127.0.0.1:5000" {
		t.Fatalf("localServiceBaseURL() = %q, want %q", got, "http://127.0.0.1:5000")
	}
}
