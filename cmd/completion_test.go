/*
 * MinIO Client (C) 2020 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"fmt"
	"testing"

	"github.com/minio/cli"
)

func TestCheckBashCompletion(t *testing.T) {
	var checkBashCompletion func(app cli.Command) error

	checkBashCompletion = func(app cli.Command) error {
		if len(app.Subcommands) == 0 {
			if app.BashComplete == nil {
				return fmt.Errorf("missing bash completion func")
			}
			return nil
		}
		for _, app := range app.Subcommands {
			if e := checkBashCompletion(app); e != nil {
				return fmt.Errorf("%s, %v", app.Name, e)
			}
		}

		return nil
	}

	for _, app := range appCmds {
		if e := checkBashCompletion(app); e != nil {
			t.Fatalf("%s, %v", app.Name, e)
		}
	}
}
