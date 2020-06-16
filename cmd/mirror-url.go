/*
 * MinIO Client (C) 2015, 2016 MinIO, Inc.
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
	"runtime"
	"strings"
	"time"

	"github.com/minio/cli"
	"github.com/minio/minio/pkg/wildcard"
)

//
//   * MIRROR ARGS - VALID CASES
//   =========================
//   mirror(d1..., d2) -> []mirror(d1/f, d2/d1/f)

// checkMirrorSyntax(URLs []string)
func checkMirrorSyntax(ctx *cli.Context, encKeyDB map[string][]prefixSSEPair) {
	if len(ctx.Args()) != 2 {
		cli.ShowCommandHelpAndExit(ctx, "mirror", 1) // last argument is exit code.
	}

	// extract URLs.
	URLs := ctx.Args()
	srcURL := URLs[0]
	tgtURL := URLs[1]

	if ctx.Bool("force") && ctx.Bool("remove") {
		errorIf(errInvalidArgument().Trace(URLs...), "`--force` is deprecated please use `--overwrite` instead with `--remove` for the same functionality.")
	} else if ctx.Bool("force") {
		errorIf(errInvalidArgument().Trace(URLs...), "`--force` is deprecated please use `--overwrite` instead for the same functionality.")
	}

	tgtClientURL := newClientURL(tgtURL)
	if tgtClientURL.Host != "" {
		if tgtClientURL.Path == string(tgtClientURL.Separator) {
			fatalIf(errInvalidArgument().Trace(tgtURL),
				fmt.Sprintf("Target `%s` does not contain bucket name.", tgtURL))
		}
	}

	_, expandedSourcePath, _ := mustExpandAlias(srcURL)
	srcClient := newClientURL(expandedSourcePath)
	_, expandedTargetPath, _ := mustExpandAlias(tgtURL)
	destClient := newClientURL(expandedTargetPath)

	// Mirror with preserve option on windows
	// only works for object storage to object storage
	if runtime.GOOS == "windows" && ctx.Bool("a") {
		if srcClient.Type == fileSystem || destClient.Type == fileSystem {
			errorIf(errInvalidArgument(), "Preserve functionality on windows support object storage to object storage transfer only.")
		}
	}

	/****** Generic rules *******/
	if !ctx.Bool("watch") {
		_, srcContent, err := url2Stat(srcURL, "", false, encKeyDB)
		// incomplete uploads are not necessary for copy operation, no need to verify for them.
		isIncomplete := false
		if err != nil && !isURLPrefixExists(srcURL, isIncomplete) {
			errorIf(err.Trace(srcURL), "Unable to stat source `"+srcURL+"`.")
		}

		if err == nil {
			if !srcContent.Type.IsDir() {
				fatalIf(errInvalidArgument().Trace(srcContent.URL.String(), srcContent.Type.String()), fmt.Sprintf("Source `%s` is not a folder. Only folders are supported by mirror command.", srcURL))
			}
		}
	}

	timeRef := ctx.String("time-reference")
	if timeRef != "" {
		if !strings.HasPrefix(timeRef, "after ") && !strings.HasPrefix(timeRef, "before ") {
			fatalIf(errInvalidArgument(), "Missing 'after' or 'before' keyword in time reference flag.")
		}

		timeRef = strings.TrimPrefix(timeRef, "after")
		timeRef = strings.TrimPrefix(timeRef, "before")
		timeRef = strings.TrimSpace(timeRef)

		_, err := time.Parse(time.RFC3339, timeRef)
		if err != nil {
			fatalIf(errInvalidArgument(), fmt.Sprintf("Time reference `%s` cannot be parsed.", timeRef))
		}
	}

}

func matchExcludeOptions(excludeOptions []string, srcSuffix string) bool {
	for _, pattern := range excludeOptions {
		if wildcard.Match(pattern, srcSuffix) {
			return true
		}
	}
	return false
}

func deltaSourceTarget(sourceURL, targetURL string, isFake, isOverwrite, isRemove bool, after bool, timeRef time.Time, isMetadata bool, excludeOptions []string, URLsCh chan<- URLs, encKeyDB map[string][]prefixSSEPair) {
	// source and targets are always directories
	sourceSeparator := string(newClientURL(sourceURL).Separator)
	if !strings.HasSuffix(sourceURL, sourceSeparator) {
		sourceURL = sourceURL + sourceSeparator
	}
	targetSeparator := string(newClientURL(targetURL).Separator)
	if !strings.HasSuffix(targetURL, targetSeparator) {
		targetURL = targetURL + targetSeparator
	}

	// Extract alias and expanded URL
	sourceAlias, sourceURL, _ := mustExpandAlias(sourceURL)
	targetAlias, targetURL, _ := mustExpandAlias(targetURL)

	defer close(URLsCh)

	sourceClnt, err := newClientFromAlias(sourceAlias, sourceURL)
	if err != nil {
		URLsCh <- URLs{Error: err.Trace(sourceAlias, sourceURL)}
		return
	}

	targetClnt, err := newClientFromAlias(targetAlias, targetURL)
	if err != nil {
		URLsCh <- URLs{Error: err.Trace(targetAlias, targetURL)}
		return
	}

	// List both source and target, compare and return values through channel.
	for diffMsg := range objectDifference(sourceClnt, targetClnt, sourceURL, targetURL, after, timeRef, isMetadata) {
		if diffMsg.Error != nil {
			// Send all errors through the channel
			URLsCh <- URLs{Error: diffMsg.Error}
			continue
		}

		srcSuffix := strings.TrimPrefix(diffMsg.FirstURL, sourceURL)
		//Skip the source object if it matches the Exclude options provided
		if matchExcludeOptions(excludeOptions, srcSuffix) {
			continue
		}

		tgtSuffix := strings.TrimPrefix(diffMsg.SecondURL, targetURL)
		//Skip the target object if it matches the Exclude options provided
		if matchExcludeOptions(excludeOptions, tgtSuffix) {
			continue
		}

		switch diffMsg.Diff {
		case differInNone:
			// No difference, continue.
		case differInType:
			URLsCh <- URLs{Error: errInvalidTarget(diffMsg.SecondURL)}
		case differInSize, differInMetadata, differInMMSourceMTime:
			if !isOverwrite && !isFake {
				// Size or time or etag differs but --overwrite not set.
				URLsCh <- URLs{Error: errOverWriteNotAllowed(diffMsg.SecondURL)}
				continue
			}

			sourceSuffix := strings.TrimPrefix(diffMsg.FirstURL, sourceURL)
			// Either available only in source or size differs and force is set
			targetPath := urlJoinPath(targetURL, sourceSuffix)
			sourceContent := diffMsg.firstContent
			targetContent := &ClientContent{URL: *newClientURL(targetPath)}
			URLsCh <- URLs{
				SourceAlias:   sourceAlias,
				SourceContent: sourceContent,
				TargetAlias:   targetAlias,
				TargetContent: targetContent,
			}
		case differInFirst:
			// Only in first, always copy.
			sourceSuffix := strings.TrimPrefix(diffMsg.FirstURL, sourceURL)
			targetPath := urlJoinPath(targetURL, sourceSuffix)
			sourceContent := diffMsg.firstContent
			targetContent := &ClientContent{URL: *newClientURL(targetPath)}
			URLsCh <- URLs{
				SourceAlias:   sourceAlias,
				SourceContent: sourceContent,
				TargetAlias:   targetAlias,
				TargetContent: targetContent,
			}
		case differInSecond:
			if !isRemove && !isFake {
				continue
			}
			URLsCh <- URLs{
				TargetAlias:   targetAlias,
				TargetContent: diffMsg.secondContent,
			}
		default:
			URLsCh <- URLs{
				Error: errUnrecognizedDiffType(diffMsg.Diff).Trace(diffMsg.FirstURL, diffMsg.SecondURL),
			}
		}
	}
}

// Prepares urls that need to be copied or removed based on requested options.
func prepareMirrorURLs(sourceURL string, targetURL string, isFake, isOverwrite, isRemove bool, after bool, timeRef time.Time, isMetadata bool, excludeOptions []string, encKeyDB map[string][]prefixSSEPair) <-chan URLs {
	URLsCh := make(chan URLs)
	go deltaSourceTarget(sourceURL, targetURL, isFake, isOverwrite, isRemove, after, timeRef, isMetadata, excludeOptions, URLsCh, encKeyDB)
	return URLsCh
}
