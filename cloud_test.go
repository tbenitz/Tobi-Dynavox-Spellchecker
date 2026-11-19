package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const typoSample = "helllo wurld im tiired how are ou doingg?"
const correctedSample = "Hello world, I'm tired. How are you doing?"

func TestCloudProvidersReturnReviewForExactDraft(t *testing.T) {
	for _, id := range []string{"deepseek", "grok"} {
		t.Run(id, func(t *testing.T) {
			p, ok := providerFor(id)
			if !ok {
				t.Fatal("missing provider")
			}
			outer, _ := json.Marshal(map[string]string{"corrected": correctedSample})
			inner, _ := json.Marshal(string(outer))
			var requests int
			c := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.Method != "POST" || r.URL.String() != p.endpoint || r.Header.Get("Authorization") != "Bearer sk-test-only" {
					t.Fatal("wrong request destination or auth")
				}
				var payload map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				messages, response := "messages", fmt.Sprintf(`{"choices":[{"finish_reason":"stop","message":{"content":%s}}]}`, inner)
				if id == "grok" {
					messages = "input"
					if string(payload["store"]) != "false" || payload["text"] == nil {
						t.Fatal("Grok storage wasn't disabled or schema missing")
					}
					response = fmt.Sprintf(`{"status":"completed","output":[{"type":"reasoning"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":%s}]}]}`, inner)
				} else if !strings.Contains(string(payload["thinking"]), "disabled") || payload["response_format"] == nil {
					t.Fatal("missing DeepSeek options")
				}
				var got []cloudMessage
				var user map[string]string
				if err := json.Unmarshal(payload[messages], &got); err != nil || len(got) != 2 || got[0].Role != "system" || got[1].Role != "user" ||
					json.Unmarshal([]byte(got[1].Content), &user) != nil || len(user) != 1 || user["draft"] != typoSample || strings.Contains(got[0].Content, typoSample) {
					t.Fatal("draft wasn't isolated from the instructions or was not sent verbatim")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
			})}
			got, changes, err := spellCloud(context.Background(), c, p, "sk-test-only", typoSample, true)
			if err != nil || got != correctedSample || changes != 1 || requests != 1 {
				t.Fatalf("got %q / %d / %v / %d requests", got, changes, err, requests)
			}
		})
	}
}

func TestCloudNoConsentOrKeyNeverSends(t *testing.T) {
	c := &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthorized request was sent")
		return nil, nil
	})}
	p, _ := providerFor("deepseek")
	for _, v := range []struct {
		key, draft string
		allow      bool
	}{
		{"sk-test", typoSample, false}, {"", typoSample, true}, {"sk-test\r\n", typoSample, true}, {"sk-test", "", true},
	} {
		if _, _, err := spellCloud(context.Background(), c, p, v.key, v.draft, v.allow); err == nil {
			t.Fatal("expected rejection")
		}
	}
}

func TestCloudErrorsDontEchoProviderBody(t *testing.T) {
	p, _ := providerFor("deepseek")
	for _, code := range []int{301, 401, 402, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/" {
					t.Errorf("unexpected redirect to %s", r.URL.Path)
				}
				w.Header().Set("Location", "/other")
				w.WriteHeader(code)
				fmt.Fprint(w, "sk-sensitive draft")
			}))
			defer srv.Close()
			p.endpoint = srv.URL
			_, _, err := spellCloud(context.Background(), srv.Client(), p, "sk-test", typoSample, true)
			if err == nil || strings.Contains(err.Error(), "sk-sensitive") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestCloudInvalidOrChangedNumbersRejected(t *testing.T) {
	for _, invalid := range []string{`{}`, `{"corrected":""}`, `{"corrected":"I need 20 mL."}`, `{"corrected":"I need 2 mL."} trailing`, `{"corrected":"I need 2 mL.","notes":"new"}`, `{"corrected":"Something else","corrected":"I need 2 mL."}`} {
		if _, _, err := cloudPreview("I nede 2 mL.", invalid); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
	if got, _, err := cloudPreview("I nede 2 mL.", `{"corrected":"I need 2 mL."}`); err != nil || got != "I need 2 mL." {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, changed := range []string{`{"corrected":"It was +2 degrees."}`, `{"corrected":"It was 2 degrees."}`} {
		if _, _, err := cloudPreview("It was -2 degres.", changed); err == nil {
			t.Fatalf("accepted changed signed number: %s", changed)
		}
	}
	if _, _, err := cloudPreview("I need .5 mL.", `{"corrected":"I need 5 mL."}`); err == nil {
		t.Fatal("accepted a changed fractional value")
	}
}

func TestCloudDecodersRejectIncompleteReplies(t *testing.T) {
	for id, body := range map[string]string{
		"deepseek": `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`,
		"grok":     `{"status":"incomplete","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{}"}]}]}`,
	} {
		p, _ := providerFor(id)
		if _, err := p.extract([]byte(body)); err == nil {
			t.Fatalf("%s accepted incomplete reply", id)
		}
	}
}
