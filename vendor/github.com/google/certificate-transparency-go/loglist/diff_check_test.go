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

package loglist

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mohae/deepcopy"
)

func pprint(stringList []string) string {
	var pretty string
	if buf, err := json.MarshalIndent(stringList, "", "  "); err == nil {
		pretty = string(buf)
	} else {
		pretty = fmt.Sprintf("%v", stringList)
	}
	return pretty
}

func TestCheckOperatorsDiff(t *testing.T) {
	tests := []struct {
		name         string
		branch       LogList
		wantWarnings []string
	}{
		{
			name: "Equal",
			branch: LogList{
				Operators: []Operator{
					{ID: 0, Name: "Google"},
					{ID: 1, Name: "Bob's CT Log Shop"},
				},
				Logs: []Log{},
			},
			wantWarnings: []string{},
		},
		{
			name: "ShuffledRenamed",
			branch: LogList{
				Operators: []Operator{
					{ID: 1, Name: "Bob's CT Log Shop+"},
					{ID: 0, Name: "Google"},
				},
				Logs: []Log{},
			},
			wantWarnings: []string{},
		},
		{
			name: "Missing",
			branch: LogList{
				Operators: []Operator{
					{ID: 1, Name: "Bob's CT Log Shop"},
				},
				Logs: []Log{},
			},
			wantWarnings: []string{"Operator \"Google\" id=0 present at master log list but missing at branch."},
		},
		{
			name: "Added",
			branch: LogList{
				Operators: []Operator{
					{ID: 0, Name: "Google"},
					{ID: 1, Name: "Bob's CT Log Shop"},
					{ID: 2, Name: "Alice's CT Log Shop"},
				},
				Logs: []Log{},
			},
			wantWarnings: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wl := warningList{warnings: []string{}}
			checkMasterOpsMatchBranch(&sampleLogList, &test.branch, &wl)
			wMismatchIds := getMismatchIds(wl.warnings, test.wantWarnings)
			if len(wMismatchIds) > 0 {
				t.Errorf("checkOperators %s: got '%v', want warnings '%v'.\n %v-st/d/th warning mismatch.", test.name, wl.warnings, test.wantWarnings, wMismatchIds)
			}
		})
	}
}

func generateKeyURLMismatch(ll *LogList) Log {
	log := deepcopy.Copy(ll.Logs[0]).(Log)
	log.Key = ll.Logs[1].Key
	log.URL = ll.Logs[1].URL
	return log
}

func generateTimingsMismatch(ll *LogList) Log {
	log := deepcopy.Copy(ll.Logs[0]).(Log)
	log.MaximumMergeDelay = 86401
	log.DisqualifiedAt = 1460678400
	return log
}

func generateOperatorsDNSMismatch(ll *LogList) Log {
	log := deepcopy.Copy(ll.Logs[0]).(Log)
	log.OperatedBy = append(log.OperatedBy, 1)
	log.DNSAPIEndpoint = ll.Logs[1].DNSAPIEndpoint
	return log
}

func TestCheckLogPairEquivalence(t *testing.T) {
	tests := []struct {
		name         string
		log1         Log
		log2         Log
		wantWarnings []string
	}{
		{
			name:         "Equal",
			log1:         deepcopy.Copy(sampleLogList.Logs[0]).(Log),
			log2:         deepcopy.Copy(sampleLogList.Logs[0]).(Log),
			wantWarnings: []string{},
		},
		{
			name: "KeyURLMismatch",
			log1: deepcopy.Copy(sampleLogList.Logs[0]).(Log),
			log2: generateKeyURLMismatch(&sampleLogList),
			wantWarnings: []string{
				"Log \"Google 'Aviator' log\" and log \"Google 'Aviator' log\" have different keys.",
				"URL mismatch for logs \"Google 'Aviator' log\" and \"Google 'Aviator' log\": ct.googleapis.com/aviator/ != ct.googleapis.com/icarus/.",
			},
		},
		{
			name: "TimingsMismatch",
			log1: deepcopy.Copy(sampleLogList.Logs[0]).(Log),
			log2: generateTimingsMismatch(&sampleLogList),
			wantWarnings: []string{
				"Maximum merge delay mismatch for logs \"Google 'Aviator' log\" and \"Google 'Aviator' log\": 86400 != 86401.",
				"Disqualified-at-timing mismatch for logs \"Google 'Aviator' log\" and \"Google 'Aviator' log\": ",
			},
		},
		{
			name: "OperatorsDNSMismatch",
			log1: deepcopy.Copy(sampleLogList.Logs[0]).(Log),
			log2: generateOperatorsDNSMismatch(&sampleLogList),
			wantWarnings: []string{
				"Operators mismatch for logs \"Google 'Aviator' log\" and \"Google 'Aviator' log\".",
				"DNS API mismatch for logs \"Google 'Aviator' log\" and \"Google 'Aviator' log\": aviator.ct.googleapis.com != icarus.ct.googleapis.com.",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wl := warningList{warnings: []string{}}
			test.log1.checkEquivalence(&test.log2, &wl)
			wMismatchIds := getMismatchIds(wl.warnings, test.wantWarnings)
			if len(wMismatchIds) > 0 {
				t.Errorf("checkLogs %s: got '%v', want warnings '%v'.\n %v-st/d/th warning mismatch.", test.name, wl.warnings, test.wantWarnings, wMismatchIds)
			}
		})
	}
}

func leaveSingleLog() LogList {
	ll := deepcopy.Copy(sampleLogList).(LogList)
	ll.Logs = ll.Logs[0:1]
	return ll
}

func leaveSingleLogSingleOp() LogList {
	ll := leaveSingleLog()
	ll.Operators = ll.Operators[0:1]
	return ll
}

func messOperators() LogList {
	ll := deepcopy.Copy(sampleLogList).(LogList)
	ll.Logs[0].OperatedBy = []int{1}
	ll.Operators = ll.Operators[0:1]
	return ll
}

func swapLogsSwapOps() LogList {
	ll := deepcopy.Copy(sampleLogList).(LogList)
	ll.Logs[0] = sampleLogList.Logs[3]
	ll.Logs[3] = sampleLogList.Logs[0]
	ll.Operators[0] = sampleLogList.Operators[1]
	ll.Operators[1] = sampleLogList.Operators[0]
	return ll
}

func TestCheckBranch(t *testing.T) {
	tests := []struct {
		name         string
		branchList   LogList
		wantWarnings []string
		wantError    bool
	}{
		{
			name:         "Copy",
			branchList:   deepcopy.Copy(sampleLogList).(LogList),
			wantWarnings: []string{},
			wantError:    false,
		},
		{
			name:         "OneMatch",
			branchList:   leaveSingleLog(),
			wantWarnings: []string{},
			wantError:    false,
		},
		{
			name:       "OneMatchOperatorMiss", // Operator exclusion is restricted.
			branchList: leaveSingleLogSingleOp(),
			wantWarnings: []string{
				"Operator \"Bob's CT Log Shop\" id=1 present at master log list but missing at branch.",
			},
			wantError: true,
		},
		{
			name:         "Shuffled",
			branchList:   swapLogsSwapOps(),
			wantWarnings: []string{},
			wantError:    false,
		},
		{
			name:       "OperatorsMess",
			branchList: messOperators(),
			wantWarnings: []string{
				"Operator \"Bob's CT Log Shop\" id=1 present at master log list but missing at branch.",
				"Operators mismatch for logs \"Google 'Aviator' log\" and \"Google 'Aviator' log\".",
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wl := sampleLogList.CheckBranch(&test.branchList)
			if test.wantError != (len(wl) > 0) {
				t.Errorf("CheckBranch %s: error status mismatch.", test.name)
			}
			wMismatchIds := getMismatchIds(wl, test.wantWarnings)
			if len(wMismatchIds) > 0 {
				t.Errorf("CheckBranch %s: got '%v', want warnings '%v'.\n %v-st/d/th warning mismatch.", test.name, wl, test.wantWarnings, wMismatchIds)
			}
		})
	}
}

func getMismatchIds(got []string, want []string) []int {
	wMismatchIds := make([]int, 0)
	for i := 0; i < len(got) || i < len(want); i++ {
		if i >= len(got) || i >= len(want) || !strings.Contains(got[i], want[i]) {
			wMismatchIds = append(wMismatchIds, i)
		}
	}
	return wMismatchIds
}
