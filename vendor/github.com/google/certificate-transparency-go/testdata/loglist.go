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

package testdata

var (

	// SampleLogList is used for testing.
	SampleLogList = `{"logs":` +
		`[` +
		`{"description":"Google 'Aviator' log",` +
		`"key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE1/TMabLkDpCjiupacAlP7xNi0I1JYP8bQFAHDG1xhtolSY1l4QgNRzRrvSe8liE+NPWHdjGxfx3JhTsN9x8/6Q==",` +
		`"maximum_merge_delay":86400,` +
		`"operated_by":[0],` +
		`"url":"ct.googleapis.com/aviator/",` +
		`"final_sth":{` +
		`"tree_size":46466472,` +
		`"timestamp":1480512258330,` +
		`"sha256_root_hash":"LcGcZRsm+LGYmrlyC5LXhV1T6OD8iH5dNlb0sEJl9bA=",` +
		`"tree_head_signature":"BAMASDBGAiEA/M0Nvt77aNe+9eYbKsv6rRpTzFTKa5CGqb56ea4hnt8CIQCJDE7pL6xgAewMd5i3G1lrBWgFooT2kd3+zliEz5Rw8w=="` +
		`},` +
		`"dns_api_endpoint":"aviator.ct.googleapis.com"` +
		`},` +
		`{"description":"Google 'Icarus' log",` +
		`"key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAETtK8v7MICve56qTHHDhhBOuV4IlUaESxZryCfk9QbG9co/CqPvTsgPDbCpp6oFtyAHwlDhnvr7JijXRD9Cb2FA==",` +
		`"maximum_merge_delay":86400,` +
		`"operated_by":[0],` +
		`"url":"ct.googleapis.com/icarus/",` +
		`"dns_api_endpoint":"icarus.ct.googleapis.com"` +
		`},` +
		`{"description":"Google 'Rocketeer' log",` +
		`"key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIFsYyDzBi7MxCAC/oJBXK7dHjG+1aLCOkHjpoHPqTyghLpzA9BYbqvnV16mAw04vUjyYASVGJCUoI3ctBcJAeg==",` +
		`"maximum_merge_delay":86400,` +
		`"operated_by":[0],` +
		`"url":"ct.googleapis.com/rocketeer/",` +
		`"dns_api_endpoint":"rocketeer.ct.googleapis.com"` +
		`},` +
		`{"description":"Google 'Racketeer' log",` +
		`"key":"Hy2TPTZ2yq9ASMmMZiB9SZEUx5WNH5G0Ft5Tm9vKMcPXA+ic/Ap3gg6fXzBJR8zLkt5lQjvKMdbHYMGv7yrsZg==",` +
		`"maximum_merge_delay":86400,` +
		`"operated_by":[0],` +
		`"url":"ct.googleapis.com/racketeer/",` +
		`"dns_api_endpoint":"racketeer.ct.googleapis.com"` +
		`},` +
		`{"description":"Bob's Dubious Log",` +
		`"key":"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAECyPLhWKYYUgEc+tUXfPQB4wtGS2MNvXrjwFCCnyYJifBtd2Sk7Cu+Js9DNhMTh35FftHaHu6ZrclnNBKwmbbSA==",` +
		`"maximum_merge_delay":86400,` +
		`"operated_by":[1],` +
		`"url":"log.bob.io",` +
		`"disqualified_at":1460678400,` +
		`"dns_api_endpoint":"dubious-bob.ct.googleapis.com"` +
		`}` +
		`],` +
		`"operators":[` +
		`{"id":0,"name":"Google"},` +
		`{"id":1,"name":"Bob's CT Log Shop"}` +
		`]}`
)
