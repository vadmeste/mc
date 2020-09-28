/*
 * MinIO Client (C) 2019 MinIO, Inc.
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
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/minio/cli"
	"github.com/posener/complete"
)

// fsComplete knows how to complete file/dir names by the given path
type fsCompleteV2 struct{}

func (fs fsCompleteV2) predictPath(arg string) []string {
	dir := filepath.Dir(arg)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil
	}

	fn := filepath.Base(arg)

	var predictions []string
	for _, file := range files {
		if strings.HasPrefix(file.Name(), fn) {
			predictions = append(predictions, file.Name())
		}
	}
	return predictions
}

// predictPathWithTilde completes an FS path which starts with a `~/`
func (fs fsCompleteV2) predictPathWithTilde(arg string) []string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return nil
	}
	// Clean the home directory path
	homeDir = strings.TrimRight(homeDir, "/")

	// Replace the first occurrence of ~ with the real path and complete
	arg = strings.Replace(arg, "~", homeDir, 1)
	predictions := fs.predictPath(arg)

	// Restore ~ to avoid disturbing the completion user experience
	for i := range predictions {
		predictions[i] = strings.Replace(predictions[i], homeDir, "~", 1)
	}

	return predictions
}

func (fs fsCompleteV2) Predict(ctx *cli.Context) []string {
	arg := ctx.Args().First()
	if strings.HasPrefix(arg, "~/") {
		return fs.predictPathWithTilde(arg)
	}
	return fs.predictPath(arg)
}

func completeAdminConfigKeysV2(aliasPath string, keyPrefix string) (prediction []string) {
	// Convert alias/bucket/incompl to alias/bucket/ to list its contents
	parentDirPath := filepath.Dir(aliasPath) + "/"
	clnt, err := newAdminClient(parentDirPath)
	if err != nil {
		return nil
	}

	h, e := clnt.HelpConfigKV(globalContext, "", "", false)
	if e != nil {
		return nil
	}

	for _, hkv := range h.KeysHelp {
		if strings.HasPrefix(hkv.Key, keyPrefix) {
			prediction = append(prediction, hkv.Key)
		}
	}

	return prediction
}

// Complete S3 path. If the prediction result is only one directory,
// then recursively scans it. This is needed to satisfy posener/complete
// (look at posener/complete.PredictFiles)
func completeS3PathV2(s3Path string) (prediction []string) {

	// Convert alias/bucket/incompl to alias/bucket/ to list its contents
	parentDirPath := filepath.Dir(s3Path) + "/"
	clnt, err := newClient(parentDirPath)
	if err != nil {
		return nil
	}

	// Calculate alias from the path
	alias := splitStr(s3Path, "/", 3)[0]

	// List dirPath content and only pick elements that corresponds
	// to the path that we want to complete
	for content := range clnt.List(globalContext, ListOptions{isRecursive: false, showDir: DirFirst}) {
		cmplS3Path := alias + getKey(content)
		if content.Type.IsDir() {
			if !strings.HasSuffix(cmplS3Path, "/") {
				cmplS3Path += "/"
			}
		}
		if strings.HasPrefix(cmplS3Path, s3Path) {
			prediction = append(prediction, cmplS3Path)
		}
	}

	// If completion found only one directory, recursively scan it.
	if len(prediction) == 1 && strings.HasSuffix(prediction[0], "/") {
		prediction = append(prediction, completeS3Path(prediction[0])...)
	}

	return
}

type adminConfigCompleteV2 struct{}

func (adm adminConfigCompleteV2) Predict(a complete.Args) (prediction []string) {
	defer func() {
		sort.Strings(prediction)
	}()

	loadMcConfig = loadMcConfigFactory()
	conf, err := loadMcConfig()
	if err != nil {
		return
	}

	// We have already predicted the keys, we are done.
	if len(a.Completed) == 3 {
		return
	}

	arg := a.Last
	lastArg := a.LastCompleted
	if _, ok := conf.Aliases[filepath.Clean(a.LastCompleted)]; !ok {
		if strings.IndexByte(arg, '/') == -1 {
			// Only predict alias since '/' is not found
			for alias := range conf.Aliases {
				if strings.HasPrefix(alias, arg) {
					prediction = append(prediction, alias+"/")
				}
			}
		} else {
			prediction = completeAdminConfigKeys(arg, "")
		}
	} else {
		prediction = completeAdminConfigKeys(lastArg, arg)
	}
	return
}

// s3Complete knows how to complete an mc s3 path
type s3CompleteV2 struct {
	deepLevel int
}

func (s3 s3CompleteV2) Predict(ctx *cli.Context) (prediction []string) {
	loadMcConfig = loadMcConfigFactory()
	conf, err := loadMcConfig()
	if err != nil {
		return nil
	}

	arg := ctx.Args().First()

	if strings.IndexByte(arg, '/') == -1 {
		// Only predict alias since '/' is not found
		for alias := range conf.Aliases {
			if strings.HasPrefix(alias, arg) {
				prediction = append(prediction, alias+"/")
			}
		}
		if len(prediction) == 1 && strings.HasSuffix(prediction[0], "/") {
			prediction = append(prediction, completeS3Path(prediction[0])...)
		}
	} else {
		// Complete S3 path until the specified path deep level
		if s3.deepLevel > 0 {
			if strings.Count(arg, "/") >= s3.deepLevel {
				return []string{arg}
			}
		}
		// Predict S3 path
		prediction = completeS3Path(arg)
	}

	return
}

// aliasComplete only completes aliases
type aliasCompleteV2 struct{}

func (al aliasCompleteV2) Predict(ctx *cli.Context) []string {
	loadMcConfig = loadMcConfigFactory()
	conf, err := loadMcConfig()
	if err != nil {
		return nil
	}

	var predictions []string

	arg := ctx.Args().Get(0)
	for alias := range conf.Aliases {
		if strings.HasPrefix(alias, arg) {
			predictions = append(predictions, alias+"/")
		}
	}

	return predictions
}

type predictors interface {
	Predict(ctx *cli.Context) []string
}

func completeFn(predictors ...predictors) func(*cli.Context) {
	return func(ctx *cli.Context) {
		var predictions []string
		for _, p := range predictors {
			if p != nil {
				predictions = append(predictions, p.Predict(ctx)...)
			}
		}
		sort.Strings(predictions)
		for _, pred := range predictions {
			fmt.Println(pred)
		}
	}
}
