// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jsonclient

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/testdata"
)

func publicKeyPEMToDER(key string) []byte {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		panic("failed to decode public key PEM")
	}
	if block.Type != "PUBLIC KEY" {
		panic("PEM does not have type 'PUBLIC KEY'")
	}
	return block.Bytes
}

func TestNewJSONClient(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{
			name:    "invalid PublicKey",
			opts:    Options{PublicKey: "bogus"},
			wantErr: "no PEM block",
		},
		{
			name:    "invalid PublicKeyDER",
			opts:    Options{PublicKeyDER: []byte("bogus")},
			wantErr: "asn1: structure error",
		},
		{
			name: "RSA PublicKey",
			opts: Options{PublicKey: testdata.RsaPublicKeyPEM},
		},
		{
			name: "RSA PublicKeyDER",
			opts: Options{PublicKeyDER: publicKeyPEMToDER(testdata.RsaPublicKeyPEM)},
		},
		{
			name: "ECDSA PublicKey",
			opts: Options{PublicKey: testdata.EcdsaPublicKeyPEM},
		},
		{
			name: "ECDSA PublicKeyDER",
			opts: Options{PublicKeyDER: publicKeyPEMToDER(testdata.EcdsaPublicKeyPEM)},
		},
		{
			name:    "DSA PublicKey",
			opts:    Options{PublicKey: testdata.DsaPublicKeyPEM},
			wantErr: "Unsupported public key type",
		},
		{
			name:    "DSA PublicKeyDER",
			opts:    Options{PublicKeyDER: publicKeyPEMToDER(testdata.DsaPublicKeyPEM)},
			wantErr: "Unsupported public key type",
		},
		{
			name:    "PublicKey contains trailing garbage",
			opts:    Options{PublicKey: testdata.RsaPublicKeyPEM + "bogus"},
			wantErr: "extra data found",
		},
		{
			name:    "PublicKeyDER contains trailing garbage",
			opts:    Options{PublicKeyDER: append(publicKeyPEMToDER(testdata.RsaPublicKeyPEM), []byte("deadbeef")...)},
			wantErr: "trailing data",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := New("http://127.0.0.1", nil, test.opts)
			if err != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("New()=nil,%v; want _,nil", err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("New()=nil,%v; want _, error containing %q", err, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("New()=_,nil; want nil, error containing %q", test.wantErr)
			}
			if got == nil {
				t.Errorf("New()=nil,nil; want non-nil,nil")
			}
		})
	}
}

type TestStruct struct {
	TreeSize  int    `json:"tree_size"`
	Timestamp int    `json:"timestamp"`
	Data      string `json:"data"`
}

type TestParams struct {
	RespCode int `json:"rc"`
}

func MockServer(t *testing.T, failCount int, retryAfter int) *httptest.Server {
	t.Helper()
	mu := sync.Mutex{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/struct/path":
			fmt.Fprintf(w, `{"tree_size": 11, "timestamp": 99}`)
		case "/struct/params":
			var s TestStruct
			if r.Method == http.MethodGet {
				s.TreeSize, _ = strconv.Atoi(r.FormValue("tree_size"))
				s.Timestamp, _ = strconv.Atoi(r.FormValue("timestamp"))
				s.Data = r.FormValue("data")
			} else {
				decoder := json.NewDecoder(r.Body)
				err := decoder.Decode(&s)
				if err != nil {
					panic("Failed to decode: " + err.Error())
				}
				defer r.Body.Close()
			}
			fmt.Fprintf(w, `{"tree_size": %d, "timestamp": %d, "data": "%s"}`, s.TreeSize, s.Timestamp, s.Data)
		case "/error":
			var params TestParams
			if r.Method == http.MethodGet {
				params.RespCode, _ = strconv.Atoi(r.FormValue("rc"))
			} else {
				decoder := json.NewDecoder(r.Body)
				err := decoder.Decode(&params)
				if err != nil {
					panic("Failed to decode: " + err.Error())
				}
				defer r.Body.Close()
			}
			http.Error(w, "error page", params.RespCode)
		case "/malformed":
			fmt.Fprintf(w, `{"tree_size": 11, "timestamp": 99`) // no closing }
		case "/retry":
			if failCount > 0 {
				failCount--
				if retryAfter != 0 {
					if retryAfter > 0 {
						w.Header().Add("Retry-After", strconv.Itoa(retryAfter))
					}
					w.WriteHeader(http.StatusServiceUnavailable)
				} else {
					w.WriteHeader(http.StatusRequestTimeout)
				}
			} else {
				fmt.Fprintf(w, `{"tree_size": 11, "timestamp": 99}`)
			}
		case "/retry-rfc1123":
			if failCount > 0 {
				failCount--
				w.Header().Add("Retry-After", time.Now().Add(time.Duration(retryAfter)*time.Second).Format(time.RFC1123))
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				fmt.Fprintf(w, `{"tree_size": 11, "timestamp": 99}`)
			}
		default:
			t.Fatalf("Unhandled URL path: %s", r.URL.Path)
		}
	}))
}

func TestGetAndParse(t *testing.T) {
	tests := []struct {
		uri        string
		params     map[string]string
		wantStatus int
		want       TestStruct
		wantErr    string
	}{
		{uri: "/short%", wantErr: "invalid URL escape"},
		{uri: "/malformed", wantStatus: http.StatusOK, wantErr: "unexpected EOF"},
		{uri: "/error", params: map[string]string{"rc": "404"}, wantErr: "404 Not Found"},
		{uri: "/error", params: map[string]string{"rc": "403"}, wantErr: "403 Forbidden"},
		{uri: "/struct/path", wantStatus: http.StatusOK, want: TestStruct{11, 99, ""}},
		{
			uri:        "/struct/params",
			params:     map[string]string{"tree_size": "42", "timestamp": "88", "data": "abcd"},
			wantStatus: http.StatusOK,
			want:       TestStruct{42, 88, "abcd"},
		},
	}

	ts := MockServer(t, -1, 0)
	defer ts.Close()

	logClient, err := New(ts.URL, &http.Client{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, test := range tests {
		var got TestStruct
		httpRsp, body, err := logClient.GetAndParse(ctx, test.uri, test.params, &got)
		var gotStatus int
		if httpRsp != nil {
			gotStatus = httpRsp.StatusCode
		} else if rspErr, ok := err.(RspError); ok {
			gotStatus = rspErr.StatusCode
		}

		if err != nil {
			if len(test.wantErr) == 0 {
				t.Errorf("GetAndParse(%q)=_,_,%q; want _, _, nil", test.uri, err.Error())
			} else if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("GetAndParse(%q)=_,_,%q; want _, _, error containing %q", test.uri, err.Error(), test.wantErr)
			}
			continue
		}

		if len(test.wantErr) > 0 {
			t.Errorf("GetAndParse(%q)=%+v,_,nil; want error matching %q", test.uri, got, test.wantErr)
		}
		if gotStatus != test.wantStatus {
			t.Errorf("GetAndParse('%s') got status %d; want %d", test.uri, gotStatus, test.wantStatus)
		}

		if body == nil {
			t.Errorf("GetAndParse(%q)=_,nil,_; want _,non-nil,_", test.uri)
		}
		if test.wantStatus == http.StatusOK {
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("GetAndParse(%q)=%+v,_,nil; want %+v", test.uri, got, test.want)
			}
		}
	}
}

func TestPostAndParse(t *testing.T) {
	tests := []struct {
		uri        string
		request    interface{}
		wantStatus int
		want       TestStruct
		wantErr    string
	}{
		{uri: "/short%", wantErr: "invalid URL escape"},
		{uri: "/struct/params", request: json.Number(`invalid`), wantErr: "invalid number literal"},
		{uri: "/malformed", wantStatus: http.StatusOK, wantErr: "unexpected end of JSON"},
		{uri: "/error", request: TestParams{RespCode: 404}, wantStatus: http.StatusNotFound},
		{uri: "/error", request: TestParams{RespCode: 403}, wantStatus: http.StatusForbidden},
		{uri: "/struct/path", wantStatus: http.StatusOK, want: TestStruct{11, 99, ""}},
		{
			uri:        "/struct/params",
			wantStatus: http.StatusOK,
			request:    TestStruct{42, 88, "abcd"},
			want:       TestStruct{42, 88, "abcd"},
		},
	}

	ts := MockServer(t, -1, 0)
	defer ts.Close()

	logClient, err := New(ts.URL, &http.Client{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, test := range tests {
		var got TestStruct
		httpRsp, body, err := logClient.PostAndParse(ctx, test.uri, test.request, &got)
		var gotStatus int
		if httpRsp != nil {
			gotStatus = httpRsp.StatusCode
		} else if rspErr, ok := err.(RspError); ok {
			gotStatus = rspErr.StatusCode
		}

		if err != nil {
			if len(test.wantErr) == 0 {
				t.Errorf("PostAndParse(%q)=_,_,%q; want _, _, nil", test.uri, err.Error())
			} else if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("PostAndParse(%q)=nil,%q; want error matching %q", test.uri, err.Error(), test.wantErr)
			}
			continue
		}

		if len(test.wantErr) > 0 {
			t.Errorf("PostAndParse(%q)=%+v,nil; want error matching %q", test.uri, got, test.wantErr)
		}
		if gotStatus != test.wantStatus {
			t.Errorf("PostAndParse('%s') got status %d; want %d", test.uri, gotStatus, test.wantStatus)
		}
		if body == nil {
			t.Errorf("PostAndParse(%q)=_,nil,_; want _,non-nil,_ ", test.uri)
		}
		if test.wantStatus == http.StatusOK {
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("PostAndParse(%q)=%+v,nil; want %+v", test.uri, got, test.want)
			}
		}
	}
}

// mockBackoff is not safe for concurrent usage
type mockBackoff struct {
	override time.Duration
}

func (mb *mockBackoff) set(o *time.Duration) time.Duration {
	if o != nil {
		mb.override = *o
	}
	return 0
}
func (mb *mockBackoff) decreaseMultiplier() {}
func (mb *mockBackoff) until() time.Time    { return time.Time{} }

func TestPostAndParseWithRetry(t *testing.T) {
	tests := []struct {
		uri             string
		request         interface{}
		deadlineSecs    int // -1 indicates no deadline
		retryAfter      int // -1 indicates generate 503 with no Retry-After
		failCount       int
		wantErr         string
		expectedBackoff time.Duration // 0 indicates no expected backoff override set
	}{
		{
			uri:             "/error",
			request:         TestParams{RespCode: 418},
			deadlineSecs:    -1,
			retryAfter:      0,
			failCount:       0,
			wantErr:         "teapot",
			expectedBackoff: 0,
		},
		{
			uri:             "/short%",
			request:         nil,
			deadlineSecs:    0,
			retryAfter:      0,
			failCount:       0,
			wantErr:         "deadline exceeded",
			expectedBackoff: 0,
		},
		{
			uri:             "/retry",
			request:         nil,
			deadlineSecs:    -1,
			retryAfter:      0,
			failCount:       1,
			wantErr:         "",
			expectedBackoff: 0,
		},
		{
			uri:             "/retry",
			request:         nil,
			deadlineSecs:    -1,
			retryAfter:      5,
			failCount:       1,
			wantErr:         "",
			expectedBackoff: 5 * time.Second,
		},
		{
			uri:             "/retry-rfc1123",
			request:         nil,
			deadlineSecs:    -1,
			retryAfter:      5,
			failCount:       1,
			wantErr:         "",
			expectedBackoff: 5 * time.Second,
		},
	}
	for _, test := range tests {
		ts := MockServer(t, test.failCount, test.retryAfter)
		defer ts.Close()

		logClient, err := New(ts.URL, &http.Client{}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		mb := mockBackoff{}
		logClient.backoff = &mb
		ctx := context.Background()
		if test.deadlineSecs >= 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(time.Duration(test.deadlineSecs)*time.Second))
			defer cancel()
		}

		var got TestStruct
		httpRsp, _, err := logClient.PostAndParseWithRetry(ctx, test.uri, test.request, &got)
		if test.wantErr != "" {
			if err == nil {
				t.Errorf("PostAndParseWithRetry()=%+v,nil; want error %q", got, test.wantErr)
			} else if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("PostAndParseWithRetry()=nil,%q; want error %q", err.Error(), test.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("PostAndParseWithRetry()=nil,%q; want no error", err.Error())
		} else if httpRsp.StatusCode != http.StatusOK {
			t.Errorf("PostAndParseWithRetry() got status %d; want OK(404)", httpRsp.StatusCode)
		}
		if test.expectedBackoff > 0 && !fuzzyDurationEquals(test.expectedBackoff, mb.override, time.Second) {
			t.Errorf("Unexpected backoff override set: got: %s, wanted: %s", mb.override, test.expectedBackoff)
		}
	}
}

func TestContextRequired(t *testing.T) {
	ts := MockServer(t, -1, 0)
	defer ts.Close()

	logClient, err := New(ts.URL, &http.Client{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var result TestStruct
	_, _, err = logClient.GetAndParse(nil, "/struct/path", nil, &result)
	if err == nil {
		t.Errorf("GetAndParse() succeeded with empty Context")
	}
	_, _, err = logClient.PostAndParse(nil, "/struct/path", nil, &result)
	if err == nil {
		t.Errorf("PostAndParse() succeeded with empty Context")
	}
	_, _, err = logClient.PostAndParseWithRetry(nil, "/struct/path", nil, &result)
	if err == nil {
		t.Errorf("PostAndParseWithRetry() succeeded with empty Context")
	}
}

func TestCancelledContext(t *testing.T) {
	ts := MockServer(t, -1, 0)
	defer ts.Close()
	logClient, err := New(ts.URL, &http.Client{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var result TestStruct
	_, _, err = logClient.GetAndParse(ctx, "/struct/path", nil, &result)
	if err != context.Canceled {
		t.Errorf("GetAndParse() = (_,_,%v), want %q", err, context.Canceled)
	}
	_, _, err = logClient.PostAndParse(ctx, "/struct/path", nil, &result)
	if err != context.Canceled {
		t.Errorf("PostAndParse() = (_,_,%v), want %q", err, context.Canceled)
	}
	_, _, err = logClient.PostAndParseWithRetry(ctx, "/struct/path", nil, &result)
	if err != context.Canceled {
		t.Errorf("PostAndParseWithRetry() = (_,_,%v), want %q", err, context.Canceled)
	}
}
