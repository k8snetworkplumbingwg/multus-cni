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
	"bytes"
	"fmt"
	"reflect"
	"sort"

	ct "github.com/google/certificate-transparency-go"
)

type warningList struct {
	warnings []string
}

func (wl *warningList) addWarning(w string) {
	if w != "" {
		wl.warnings = append(wl.warnings, w)
	}
}

// checkMasterOpsMatchBranch checks operator IDs set of branch is equal to or
// wider than master one. No restriction on description mismatches.
func checkMasterOpsMatchBranch(master *LogList, branch *LogList, wl *warningList) {
	masterOps := master.OperatorIDSet()
	branchOps := branch.OperatorIDSet()
	for opID := range masterOps {
		if branchOps[opID] == "" {
			wl.addWarning(fmt.Sprintf(
				"Operator %q id=%d present at master log list but missing at branch.",
				masterOps[opID], opID))
		}
	}
}

// checkEquivalence: whether 2 logs are functionally identical.
func (log1 *Log) checkEquivalence(log2 *Log, wl *warningList) {
	// Description and STH comparison are omitted.
	if !bytes.Equal(log1.Key, log2.Key) {
		wl.addWarning(fmt.Sprintf(
			"Log %q and log %q have different keys.",
			log1.Description, log2.Description))
	}
	if log1.MaximumMergeDelay != log2.MaximumMergeDelay {
		wl.addWarning(fmt.Sprintf(
			"Maximum merge delay mismatch for logs %q and %q: %d != %d.",
			log1.Description, log2.Description, log1.MaximumMergeDelay,
			log2.MaximumMergeDelay))
	}
	// Strong assumption: operators IDs are semantically same across logs.
	log1Ops := log1.OperatedBy
	log2Ops := log2.OperatedBy
	sort.IntSlice(log1Ops).Sort()
	sort.IntSlice(log2Ops).Sort()
	if !reflect.DeepEqual(log1Ops, log2Ops) {
		wl.addWarning(fmt.Sprintf(
			"Operators mismatch for logs %q and %q.",
			log1.Description, log2.Description))
	}
	if log1.URL != log2.URL {
		wl.addWarning(fmt.Sprintf(
			"URL mismatch for logs %q and %q: %s != %s.",
			log1.Description, log2.Description, log1.URL, log2.URL))
	}
	if log1.DisqualifiedAt != log2.DisqualifiedAt {
		wl.addWarning(fmt.Sprintf(
			"Disqualified-at-timing mismatch for logs %q and %q: %v != %v.",
			log1.Description, log2.Description,
			ct.TimestampToTime(uint64(log1.DisqualifiedAt)),
			ct.TimestampToTime(uint64(log2.DisqualifiedAt))))
	}
	if log1.DNSAPIEndpoint != log2.DNSAPIEndpoint {
		wl.addWarning(fmt.Sprintf(
			"DNS API mismatch for logs %q and %q: %s != %s.",
			log1.Description, log2.Description, log1.DNSAPIEndpoint,
			log2.DNSAPIEndpoint))
	}
}

// checkMasterLogsMatchBranch checks whether logs present at branched-list
// either have equivalent key matched entry at master-list or are absent from
// master.
func checkMasterLogsMatchBranch(master *LogList, branch *LogList, wl *warningList) {
	for _, log := range branch.Logs {
		if masterEntry := master.FindLogByKey(log.Key); masterEntry != nil {
			masterEntry.checkEquivalence(&log, wl)
		}
	}
}

// CheckBranch checks edited version of LogList against a master one for edit
// restrictions: consistency across operators, matching functionality of mutual
// logs.
// Returns slice of warnings if any.
func (master *LogList) CheckBranch(branch *LogList) []string {
	w := &warningList{warnings: []string{}}
	checkMasterOpsMatchBranch(master, branch, w)
	checkMasterLogsMatchBranch(master, branch, w)
	return w.warnings
}
