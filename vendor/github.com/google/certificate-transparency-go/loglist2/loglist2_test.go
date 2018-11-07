// Copyright 2018 Google Inc. All Rights Reserved.
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

package loglist2

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

var sampleLogList = LogList{
	Operators: map[string]*Operator{
		"Google": {
			Email: []string{"google-ct-logs@googlegroups.com"},
			Logs: map[string]*Log{
				"google_aviator": {
					Description: []string{"Google 'Aviator' log"},
					LogID:       deb64("aPaY+B9kgr46jO65KB1M/HFRXWeT1ETRCmesu09P+8Q="),
					Key:         deb64("MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE1/TMabLkDpCjiupacAlP7xNi0I1JYP8bQFAHDG1xhtolSY1l4QgNRzRrvSe8liE+NPWHdjGxfx3JhTsN9x8/6Q=="),
					URL:         "https://ct.googleapis.com/aviator/",
					MMD:         86400,
					State: &LogStates{
						Frozen: &FrozenLogState{
							LogState: LogState{Timestamp: time.Unix(1480512258, 330000000).UTC()},
							FinalTreeHead: TreeHead{
								TreeSize:       46466472,
								SHA256RootHash: deb64("LcGcZRsm+LGYmrlyC5LXhV1T6OD8iH5dNlb0sEJl9bA="),
							},
						},
					},
					DNS: "aviator.ct.googleapis.com",
				},
				"google_icarus": {
					Description: []string{"Google 'Icarus' log"},
					LogID:       deb64("KTxRllTIOWW6qlD8WAfUt2+/WHopctykwwz05UVH9Hg="),
					Key:         deb64("MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAETtK8v7MICve56qTHHDhhBOuV4IlUaESxZryCfk9QbG9co/CqPvTsgPDbCpp6oFtyAHwlDhnvr7JijXRD9Cb2FA=="),
					URL:         "https://ct.googleapis.com/icarus/",
					MMD:         86400,
					DNS:         "icarus.ct.googleapis.com",
				},
				"google_racketeer": {
					Description: []string{"Google 'Racketeer' log"},
					LogID:       deb64("7kEv4llINIlh4vPgjGgugT7A/3cLbXUXF2OvMBT/l2g="),
					// Key value chosed to have a hash that starts ee4... (specifically ee412fe25948348961e2f3e08c682e813ec0ff770b6d75171763af3014ff9768)
					Key: deb64("Hy2TPTZ2yq9ASMmMZiB9SZEUx5WNH5G0Ft5Tm9vKMcPXA+ic/Ap3gg6fXzBJR8zLkt5lQjvKMdbHYMGv7yrsZg=="),
					URL: "https://ct.googleapis.com/racketeer/",
					MMD: 86400,
					DNS: "racketeer.ct.googleapis.com",
				},
				"google_rocketeer": {
					Description: []string{"Google 'Rocketeer' log"},
					LogID:       deb64("7ku9t3XOYLrhQmkfq+GeZqMPfl+wctiDAMR7iXqo/cs="),
					Key:         deb64("MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIFsYyDzBi7MxCAC/oJBXK7dHjG+1aLCOkHjpoHPqTyghLpzA9BYbqvnV16mAw04vUjyYASVGJCUoI3ctBcJAeg=="),
					URL:         "https://ct.googleapis.com/rocketeer/",
					MMD:         86400,
					DNS:         "rocketeer.ct.googleapis.com",
				},
			},
		},
		"Bob's CT Log Shop": {
			Email: []string{"bob@example.com"},
			Logs: map[string]*Log{
				"bob_dubious": {
					Description: []string{"Bob's Dubious Log"},
					LogID:       deb64("zbUXm3/BwEb+6jETaj+PAC5hgvr4iW/syLL1tatgSQA="),
					Key:         deb64("MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAECyPLhWKYYUgEc+tUXfPQB4wtGS2MNvXrjwFCCnyYJifBtd2Sk7Cu+Js9DNhMTh35FftHaHu6ZrclnNBKwmbbSA=="),
					URL:         "log.bob.io",
					MMD:         86400,
					State: &LogStates{
						Retired: &LogState{
							Timestamp: time.Unix(1460678400, 0).UTC(),
						},
					},
					DNS: "dubious-bob.ct.googleapis.com",
				},
			},
		},
	},
}

func TestJSONMarshal(t *testing.T) {
	var tests = []struct {
		name          string
		in            LogList
		want, wantErr string
	}{
		{
			name: "MultiValid",
			in:   sampleLogList,
			want: `{"operators":{` +
				`"Bob's CT Log Shop":{"email":["bob@example.com"],"logs":{` +
				`"bob_dubious":{"description":["Bob's Dubious Log"],"log_id":"zbUXm3/BwEb+6jETaj+PAC5hgvr4iW/syLL1tatgSQA=","key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAECyPLhWKYYUgEc+tUXfPQB4wtGS2MNvXrjwFCCnyYJifBtd2Sk7Cu+Js9DNhMTh35FftHaHu6ZrclnNBKwmbbSA==","url":"log.bob.io","dns":"dubious-bob.ct.googleapis.com","mmd":86400,"state":{"retired":{"timestamp":"2016-04-15T00:00:00Z"}}}}},` +
				`"Google":{"email":["google-ct-logs@googlegroups.com"],"logs":{` +
				`"google_aviator":{"description":["Google 'Aviator' log"],"log_id":"aPaY+B9kgr46jO65KB1M/HFRXWeT1ETRCmesu09P+8Q=","key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE1/TMabLkDpCjiupacAlP7xNi0I1JYP8bQFAHDG1xhtolSY1l4QgNRzRrvSe8liE+NPWHdjGxfx3JhTsN9x8/6Q==","url":"https://ct.googleapis.com/aviator/","dns":"aviator.ct.googleapis.com","mmd":86400,"state":{"frozen":{"timestamp":"2016-11-30T13:24:18.33Z","final_tree_head":{"sha256_root_hash":"LcGcZRsm+LGYmrlyC5LXhV1T6OD8iH5dNlb0sEJl9bA=","tree_size":46466472}}}},` +
				`"google_icarus":{"description":["Google 'Icarus' log"],"log_id":"KTxRllTIOWW6qlD8WAfUt2+/WHopctykwwz05UVH9Hg=","key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAETtK8v7MICve56qTHHDhhBOuV4IlUaESxZryCfk9QbG9co/CqPvTsgPDbCpp6oFtyAHwlDhnvr7JijXRD9Cb2FA==","url":"https://ct.googleapis.com/icarus/","dns":"icarus.ct.googleapis.com","mmd":86400},` +
				`"google_racketeer":{"description":["Google 'Racketeer' log"],"log_id":"7kEv4llINIlh4vPgjGgugT7A/3cLbXUXF2OvMBT/l2g=","key":"Hy2TPTZ2yq9ASMmMZiB9SZEUx5WNH5G0Ft5Tm9vKMcPXA+ic/Ap3gg6fXzBJR8zLkt5lQjvKMdbHYMGv7yrsZg==","url":"https://ct.googleapis.com/racketeer/","dns":"racketeer.ct.googleapis.com","mmd":86400},` +
				`"google_rocketeer":{"description":["Google 'Rocketeer' log"],"log_id":"7ku9t3XOYLrhQmkfq+GeZqMPfl+wctiDAMR7iXqo/cs=","key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIFsYyDzBi7MxCAC/oJBXK7dHjG+1aLCOkHjpoHPqTyghLpzA9BYbqvnV16mAw04vUjyYASVGJCUoI3ctBcJAeg==","url":"https://ct.googleapis.com/rocketeer/","dns":"rocketeer.ct.googleapis.com","mmd":86400}}}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(&test.in)
			if err != nil {
				if test.wantErr == "" {
					t.Errorf("json.Marshal()=nil,%v; want _,nil", err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("json.Marshal()=nil,%v; want nil,err containing %q", err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Errorf("json.Marshal()=%q,nil; want nil,err containing %q", got, test.wantErr)
			}
			if string(got) != test.want {
				t.Logf("json.Marshal()=%q,nil; want %q", got, test.want)
				t.Fail()
			}
		})
	}
}

func TestFindLogByName(t *testing.T) {
	var tests = []struct {
		name, in string
		want     int
	}{
		{name: "Single", in: "Dubious", want: 1},
		{name: "SingleDifferentCase", in: "DUBious", want: 1},
		{name: "Multiple", in: "Google", want: 4},
		{name: "None", in: "Llamalog", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sampleLogList.FindLogByName(test.in)
			if len(got) != test.want {
				t.Errorf("len(FindLogByName(%q)=%d, want %d", test.in, len(got), test.want)
			}
		})
	}
}

func TestFindLogByURL(t *testing.T) {
	var tests = []struct {
		name, in, want string
	}{
		{name: "NotFound", in: "nowhere.com"},
		{name: "Found//", in: "https://ct.googleapis.com/icarus/", want: "Google 'Icarus' log"},
		{name: "Found./", in: "https://ct.googleapis.com/icarus", want: "Google 'Icarus' log"},
		{name: "Found/.", in: "log.bob.io/", want: "Bob's Dubious Log"},
		{name: "Found..", in: "log.bob.io", want: "Bob's Dubious Log"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := sampleLogList.FindLogByURL(test.in)
			got := ""
			if log != nil {
				got = log.Description[0]
			}
			if got != test.want {
				t.Errorf("FindLogByURL(%q)=%q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestFindLogByKeyhash(t *testing.T) {
	var tests = []struct {
		name string
		in   []byte
		want string
	}{
		{
			name: "NotFound",
			in:   []byte{0xaa, 0xbb, 0xcc},
		},
		{
			name: "FoundRocketeer",
			in: []byte{
				0xee, 0x4b, 0xbd, 0xb7, 0x75, 0xce, 0x60, 0xba, 0xe1, 0x42, 0x69, 0x1f, 0xab, 0xe1, 0x9e, 0x66,
				0xa3, 0x0f, 0x7e, 0x5f, 0xb0, 0x72, 0xd8, 0x83, 0x00, 0xc4, 0x7b, 0x89, 0x7a, 0xa8, 0xfd, 0xcb,
			},
			want: "Google 'Rocketeer' log",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var hash [sha256.Size]byte
			copy(hash[:], test.in)
			log := sampleLogList.FindLogByKeyHash(hash)
			got := ""
			if log != nil {
				got = log.Description[0]
			}
			if got != test.want {
				t.Errorf("FindLogByKeyHash(%x)=%q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestFindLogByKeyhashPrefix(t *testing.T) {
	var tests = []struct {
		name, in string
		want     []string
	}{
		{
			name: "NotFound",
			in:   "aabbcc",
			want: []string{},
		},
		{
			name: "FoundRocketeer",
			in:   "ee4b",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundRocketeerOdd",
			in:   "ee4bb",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundMultiple",
			in:   "ee4",
			want: []string{"Google 'Racketeer' log", "Google 'Rocketeer' log"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs := sampleLogList.FindLogByKeyHashPrefix(test.in)
			got := make([]string, len(logs))
			for i, log := range logs {
				got[i] = log.Description[0]
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("FindLogByKeyHash(%x)=%q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestFindLogByKey(t *testing.T) {
	var tests = []struct {
		name string
		in   []byte
		want string
	}{
		{
			name: "NotFound",
			in:   []byte{0xaa, 0xbb, 0xcc},
		},
		{
			name: "FoundRocketeer",
			in:   deb64("MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIFsYyDzBi7MxCAC/oJBXK7dHjG+1aLCOkHjpoHPqTyghLpzA9BYbqvnV16mAw04vUjyYASVGJCUoI3ctBcJAeg=="),
			want: "Google 'Rocketeer' log",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := sampleLogList.FindLogByKey(test.in)
			got := ""
			if log != nil {
				got = log.Description[0]
			}
			if got != test.want {
				t.Errorf("FindLogByKey(%x)=%q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestFuzzyFindLog(t *testing.T) {
	var tests = []struct {
		name, in string
		want     []string
	}{
		{
			name: "NotFound",
			in:   "aabbcc",
			want: []string{},
		},
		{
			name: "FoundByKey64",
			in:   "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIFsYyDzBi7MxCAC/oJBXK7dHjG+1aLCOkHjpoHPqTyghLpzA9BYbqvnV16mAw04vUjyYASVGJCUoI3ctBcJAeg==",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByKeyHex",
			in:   "3059301306072a8648ce3d020106082a8648ce3d03010703420004205b18c83cc18bb3310800bfa090572bb7478c6fb568b08e9078e9a073ea4f28212e9cc0f4161baaf9d5d7a980c34e2f523c9801254624252823772d05c2407a",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByKeyHashHex",
			in:   " ee 4b bd b7 75 ce 60 ba e1 42 69 1f ab e1 9e 66 a3 0f 7e 5f b0 72 d8 83 00 c4 7b 89 7a a8 fd cb",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByKeyHashHexPrefix",
			in:   "ee4bbdb7",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByKeyHash64",
			in:   "7ku9t3XOYLrhQmkfq+GeZqMPfl+wctiDAMR7iXqo/cs=",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByName",
			in:   "Rocketeer",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByNameDifferentCase",
			in:   "rocketeer",
			want: []string{"Google 'Rocketeer' log"},
		},
		{
			name: "FoundByURL",
			in:   "https://ct.googleapis.com/rocketeer",
			want: []string{"Google 'Rocketeer' log"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs := sampleLogList.FuzzyFindLog(test.in)
			got := make([]string, len(logs))
			for i, log := range logs {
				got[i] = log.Description[0]
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("FuzzyFindLog(%q)=%v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestStripInternalSpace(t *testing.T) {
	var tests = []struct {
		in   string
		want string
	}{
		{in: "xyz", want: "xyz"},
		{in: "x y z", want: "xyz"},
		{in: "x  yz  ", want: "xyz"},
		{in: " xyz ", want: "xyz"},
		{in: "xy\t\tz", want: "xyz"},
	}

	for _, test := range tests {
		got := stripInternalSpace(test.in)
		if got != test.want {
			t.Errorf("stripInternalSpace(%q)=%q, want %q", test.in, got, test.want)
		}
	}
}

func deb64(b string) []byte {
	data, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		panic(fmt.Sprintf("hard-coded test data failed to decode: %v", err))
	}
	return data
}
