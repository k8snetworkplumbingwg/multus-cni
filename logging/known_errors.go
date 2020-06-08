// Copyright (c) 2018 Intel Corporation
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

package logging

import (
  "fmt"
  "strings"
)

// knownErrorPatterns is a list of all known error patterns
var knownErrorPatterns = []string{
  "error dialing DHCP daemon",
}

// getKnownErrorMessage returns
func getKnownErrorMessage(patternkey string) (string, error) {
  messages := map[string]string{
    "error dialing DHCP daemon": "please check that the dhcp cni daemon is running and is properly configured.",
  }

  if val, ok := messages[patternkey]; ok {
    return val, nil
  }

  return "", fmt.Errorf("Known error key '" + patternkey + "' does not have a message")

}

// detectKnownErrors detects the first known error given n number of stringers as passed to the logging methods
func addKnownErrorMessage(a ...interface{}) string {
  var knownerrormessage string
  var err error

  for _, eachstringer := range a {
    for _, eachknownerror := range knownErrorPatterns {
      if strings.Contains(fmt.Sprintf("%s", eachstringer), eachknownerror) {
        knownerrormessage, err = getKnownErrorMessage(eachknownerror)
        if err != nil {
          Errorf("error getting known error message: %s", err)
        }
        knownerrormessage = knownerrormessage + " (" + knownerrormessage + ") "
        continue
      }
    }
  }
  return knownerrormessage
}
