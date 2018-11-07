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

package ctfe

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keys/der"
	_ "github.com/google/trillian/crypto/keys/der/proto" // Register key handler.
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/keyspb"
)

var (
	invalidTimestamp = &timestamp.Timestamp{Nanos: int32(1e9)}

	// validSTH is an STH signed by "../testdata/ct-http-server.privkey.pem".
	validSTH = configpb.SignedTreeHead{
		TreeSize:          766,
		Timestamp:         1538659276115,
		Sha256RootHash:    mustDecodeBase64("rMWSvrYQ+n6kAmU6sMJuVV5LjoKBGK2OL719X5a+T9Y="),
		TreeHeadSignature: mustDecodeBase64("BAMASDBGAiEApo+OIdPXIVEwdnS1v5Iu1gQHaiWmCY73h28zfKMmHrYCIQD1U6qj1mKBXYIQVP52YCdaxdHJH4jDsL1JlaA+J/MqCw=="),
	}
)

func mustMarshalAny(pb proto.Message) *any.Any {
	ret, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(fmt.Sprintf("MarshalAny failed: %v", err))
	}
	return ret
}

func mustReadPublicKey(path string) *keyspb.PublicKey {
	pubKey, err := pem.ReadPublicKeyFile(path)
	if err != nil {
		panic(fmt.Sprintf("ReadPublicKeyFile(): %v", err))
	}
	ret, err := der.ToPublicProto(pubKey)
	if err != nil {
		panic(fmt.Sprintf("ToPublicProto(): %v", err))
	}
	return ret
}

func mustDecodeBase64(str string) []byte {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		panic(fmt.Sprintf("base64: DecodeString failed: %v", err))
	}
	return data
}

func TestValidateLogConfig(t *testing.T) {
	pubKey := mustReadPublicKey("../testdata/ct-http-server.pubkey.pem")
	privKey := mustMarshalAny(&keyspb.PEMKeyFile{Path: "../testdata/ct-http-server.privkey.pem", Password: "dirk"})

	corruptedSTH := validSTH
	corruptedSTH.TreeSize = 1234

	for _, tc := range []struct {
		desc    string
		cfg     configpb.LogConfig
		wantErr string
	}{
		{
			desc:    "empty-log-ID",
			wantErr: "empty log ID",
			cfg:     configpb.LogConfig{},
		},
		{
			desc:    "empty-public-key-mirror",
			wantErr: "empty public key for mirror",
			cfg:     configpb.LogConfig{LogId: 123, IsMirror: true},
		},
		{
			desc:    "empty-public-key-frozen",
			wantErr: "empty public key for frozen STH",
			cfg:     configpb.LogConfig{LogId: 123, FrozenSth: &configpb.SignedTreeHead{}},
		},
		{
			desc:    "invalid-public-key-empty",
			wantErr: "invalid public key",
			cfg: configpb.LogConfig{
				LogId:     123,
				PublicKey: &keyspb.PublicKey{},
				IsMirror:  true,
			},
		},
		{
			desc:    "invalid-public-key-abacaba",
			wantErr: "invalid public key",
			cfg: configpb.LogConfig{
				LogId:     123,
				PublicKey: &keyspb.PublicKey{Der: []byte("abacaba")},
				IsMirror:  true,
			},
		},
		{
			desc:    "empty-private-key",
			wantErr: "empty private key",
			cfg:     configpb.LogConfig{LogId: 123},
		},
		{
			desc:    "invalid-private-key",
			wantErr: "invalid private key",
			cfg: configpb.LogConfig{
				LogId:      123,
				PrivateKey: &any.Any{},
			},
		},
		{
			desc:    "unnecessary-private-key",
			wantErr: "unnecessary private key",
			cfg: configpb.LogConfig{
				LogId:      123,
				PublicKey:  pubKey,
				PrivateKey: privKey,
				IsMirror:   true,
			},
		},
		{
			desc:    "unknown-ext-key-usage-1",
			wantErr: "unknown extended key usage",
			cfg: configpb.LogConfig{
				LogId:        123,
				PrivateKey:   privKey,
				ExtKeyUsages: []string{"wrong_usage"},
			},
		},
		{
			desc:    "unknown-ext-key-usage-2",
			wantErr: "unknown extended key usage",
			cfg: configpb.LogConfig{
				LogId:        123,
				PrivateKey:   privKey,
				ExtKeyUsages: []string{"Any", "ServerAuth", "TimeStomping"},
			},
		},
		{
			desc:    "unknown-ext-key-usage-3",
			wantErr: "unknown extended key usage",
			cfg: configpb.LogConfig{
				LogId:        123,
				PrivateKey:   privKey,
				ExtKeyUsages: []string{"Any "},
			},
		},
		{
			desc:    "invalid-start-timestamp",
			wantErr: "invalid start timestamp",
			cfg: configpb.LogConfig{
				LogId:         123,
				PrivateKey:    privKey,
				NotAfterStart: invalidTimestamp,
			},
		},
		{
			desc:    "invalid-limit-timestamp",
			wantErr: "invalid limit timestamp",
			cfg: configpb.LogConfig{
				LogId:         123,
				PrivateKey:    privKey,
				NotAfterLimit: invalidTimestamp,
			},
		},
		{
			desc:    "limit-before-start",
			wantErr: "limit before start",
			cfg: configpb.LogConfig{
				LogId:         123,
				PrivateKey:    privKey,
				NotAfterStart: &timestamp.Timestamp{Seconds: 200},
				NotAfterLimit: &timestamp.Timestamp{Seconds: 100},
			},
		},
		{
			desc:    "negative-maximum-merge",
			wantErr: "negative maximum merge",
			cfg: configpb.LogConfig{
				LogId:            123,
				PrivateKey:       privKey,
				MaxMergeDelaySec: -100,
			},
		},
		{
			desc:    "negative-expected-merge",
			wantErr: "negative expected merge",
			cfg: configpb.LogConfig{
				LogId:                 123,
				PrivateKey:            privKey,
				ExpectedMergeDelaySec: -100,
			},
		},
		{
			desc:    "expected-exceeds-max",
			wantErr: "expected merge delay exceeds MMD",
			cfg: configpb.LogConfig{
				LogId:                 123,
				PrivateKey:            privKey,
				MaxMergeDelaySec:      50,
				ExpectedMergeDelaySec: 100,
			},
		},
		{
			desc:    "invalid-frozen-STH",
			wantErr: "invalid frozen STH",
			cfg: configpb.LogConfig{
				LogId:     123,
				PublicKey: pubKey,
				IsMirror:  true,
				FrozenSth: &configpb.SignedTreeHead{
					TreeSize:  10,
					Timestamp: 100500,
				},
			},
		},
		{
			desc:    "signature-verification-failed",
			wantErr: "signature verification failed",
			cfg: configpb.LogConfig{
				LogId:      123,
				PublicKey:  pubKey,
				PrivateKey: privKey,
				FrozenSth:  &corruptedSTH,
			},
		},
		{
			desc: "ok",
			cfg: configpb.LogConfig{
				LogId:      123,
				PrivateKey: privKey,
			},
		},
		{
			// Note: Substituting an arbitrary proto.Message as a PrivateKey will not
			// fail the validation because the actual key loading happens at runtime.
			// TODO(pavelkalinnikov): Decouple key protos validation and loading, and
			// make this test fail.
			desc: "ok-not-a-key",
			cfg: configpb.LogConfig{
				LogId:      123,
				PrivateKey: mustMarshalAny(&configpb.LogConfig{}),
			},
		},
		{
			desc: "ok-mirror",
			cfg: configpb.LogConfig{
				LogId:     123,
				PublicKey: pubKey,
				IsMirror:  true,
			},
		},
		{
			desc: "ok-ext-key-usages",
			cfg: configpb.LogConfig{
				LogId:        123,
				PrivateKey:   privKey,
				ExtKeyUsages: []string{"ServerAuth", "ClientAuth", "OCSPSigning"},
			},
		},
		{
			desc: "ok-start-timestamp",
			cfg: configpb.LogConfig{
				LogId:         123,
				PrivateKey:    privKey,
				NotAfterStart: &timestamp.Timestamp{Seconds: 100},
			},
		},
		{
			desc: "ok-limit-timestamp",
			cfg: configpb.LogConfig{
				LogId:         123,
				PrivateKey:    privKey,
				NotAfterLimit: &timestamp.Timestamp{Seconds: 200},
			},
		},
		{
			desc: "ok-range-timestamp",
			cfg: configpb.LogConfig{
				LogId:         123,
				PrivateKey:    privKey,
				NotAfterStart: &timestamp.Timestamp{Seconds: 300},
				NotAfterLimit: &timestamp.Timestamp{Seconds: 400},
			},
		},
		{
			desc: "ok-merge-delay",
			cfg: configpb.LogConfig{
				LogId:                 123,
				PrivateKey:            privKey,
				MaxMergeDelaySec:      86400,
				ExpectedMergeDelaySec: 7200,
			},
		},
		{
			desc: "ok-frozen-STH",
			cfg: configpb.LogConfig{
				LogId:      123,
				PublicKey:  pubKey,
				PrivateKey: privKey,
				FrozenSth:  &validSTH,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			vc, err := ValidateLogConfig(&tc.cfg)
			if len(tc.wantErr) == 0 && err != nil {
				t.Errorf("ValidateLogConfig()=%v, want nil", err)
			}
			if len(tc.wantErr) > 0 && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("ValidateLogConfig()=%v, want err containing %q", err, tc.wantErr)
			}
			if err == nil && vc == nil {
				t.Error("err and ValidatedLogConfig are both nil")
			}
			// TODO(pavelkalinnikov): Test that ValidatedLogConfig is correct.
		})
	}
}

func TestValidateLogMultiConfig(t *testing.T) {
	privKey := mustMarshalAny(&keyspb.PEMKeyFile{Path: "../testdata/ct-http-server.privkey.pem", Password: "dirk"})
	for _, tc := range []struct {
		desc    string
		cfg     configpb.LogMultiConfig
		wantErr string
	}{
		{
			desc:    "empty-backend-name",
			wantErr: "empty backend name",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{BackendSpec: "testspec"},
					},
				},
			},
		},
		{
			desc:    "empty-backend-spec",
			wantErr: "empty backend spec",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1"},
					},
				},
			},
		},
		{
			desc:    "duplicate-backend-name",
			wantErr: "duplicate backend name",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "dup", BackendSpec: "testspec"},
						{Name: "dup", BackendSpec: "testspec"},
					},
				},
			},
		},
		{
			desc:    "duplicate-backend-spec",
			wantErr: "duplicate backend spec",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec"},
						{Name: "log2", BackendSpec: "testspec"},
					},
				},
			},
		},
		{
			desc:    "invalid-log-config",
			wantErr: "log config: empty log ID",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{Prefix: "pref"},
					},
				},
			},
		},
		{
			desc:    "empty-prefix",
			wantErr: "empty prefix",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, PrivateKey: privKey, LogBackendName: "log1"},
					},
				},
			},
		},
		{
			desc:    "duplicate-prefix",
			wantErr: "duplicate prefix",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec1"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, Prefix: "pref1", PrivateKey: privKey, LogBackendName: "log1"},
						{LogId: 2, Prefix: "pref2", PrivateKey: privKey, LogBackendName: "log1"},
						{LogId: 3, Prefix: "pref1", PrivateKey: privKey, LogBackendName: "log1"},
					},
				},
			},
		},
		{
			desc:    "references-undefined-backend",
			wantErr: "references undefined backend",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 2, Prefix: "pref2", PrivateKey: privKey, LogBackendName: "log2"},
					},
				},
			},
		},
		{
			desc:    "dup-tree-id-on-same-backend",
			wantErr: "dup tree id",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec1"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, Prefix: "pref1", PrivateKey: privKey, LogBackendName: "log1"},
						{LogId: 2, Prefix: "pref2", PrivateKey: privKey, LogBackendName: "log1"},
						{LogId: 1, Prefix: "pref3", PrivateKey: privKey, LogBackendName: "log1"},
					},
				},
			},
		},
		{
			desc: "ok-all-distinct",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec1"},
						{Name: "log2", BackendSpec: "testspec2"},
						{Name: "log3", BackendSpec: "testspec3"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, Prefix: "pref1", PrivateKey: privKey, LogBackendName: "log1"},
						{LogId: 2, Prefix: "pref2", PrivateKey: privKey, LogBackendName: "log2"},
						{LogId: 3, Prefix: "pref3", PrivateKey: privKey, LogBackendName: "log3"},
					},
				},
			},
		},
		{
			desc: "ok-dup-tree-ids-on-different-backends",
			cfg: configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{
						{Name: "log1", BackendSpec: "testspec1"},
						{Name: "log2", BackendSpec: "testspec2"},
						{Name: "log3", BackendSpec: "testspec3"},
					},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, Prefix: "pref1", PrivateKey: privKey, LogBackendName: "log1"},
						{LogId: 1, Prefix: "pref2", PrivateKey: privKey, LogBackendName: "log2"},
						{LogId: 1, Prefix: "pref3", PrivateKey: privKey, LogBackendName: "log3"},
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := ValidateLogMultiConfig(&tc.cfg)
			if len(tc.wantErr) == 0 && err != nil {
				t.Fatalf("ValidateLogMultiConfig()=%v, want nil", err)
			}
			if len(tc.wantErr) > 0 && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("ValidateLogMultiConfig()=%v, want err containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestToMultiLogConfig(t *testing.T) {
	// TODO(pavelkalinnikov): Log configs in this test are not valid (they don't
	// have keys etc). In addition, we should have tests to ensure that valid log
	// configs result in valid MultiLogConfig.

	for _, tc := range []struct {
		desc string
		cfg  []*configpb.LogConfig
		want *configpb.LogMultiConfig
	}{
		{
			desc: "ok-one-config",
			cfg: []*configpb.LogConfig{
				{LogId: 1, Prefix: "test"},
			},
			want: &configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{{Name: "default", BackendSpec: "spec"}},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, Prefix: "test", LogBackendName: "default"},
					},
				},
			},
		},
		{
			desc: "ok-three-configs",
			cfg: []*configpb.LogConfig{
				{LogId: 1, Prefix: "test1"},
				{LogId: 2, Prefix: "test2"},
				{LogId: 3, Prefix: "test3"},
			},
			want: &configpb.LogMultiConfig{
				Backends: &configpb.LogBackendSet{
					Backend: []*configpb.LogBackend{{Name: "default", BackendSpec: "spec"}},
				},
				LogConfigs: &configpb.LogConfigSet{
					Config: []*configpb.LogConfig{
						{LogId: 1, Prefix: "test1", LogBackendName: "default"},
						{LogId: 2, Prefix: "test2", LogBackendName: "default"},
						{LogId: 3, Prefix: "test3", LogBackendName: "default"},
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := ToMultiLogConfig(tc.cfg, "spec")
			if !proto.Equal(got, tc.want) {
				t.Errorf("TestToMultiLogConfig()=%v, want %v", got, tc.want)
			}
		})
	}
}
