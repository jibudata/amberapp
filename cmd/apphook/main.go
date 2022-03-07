/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"path/filepath"

	"github.com/jibudata/amberapp/pkg/cmd"
	"github.com/jibudata/amberapp/pkg/cmd/apphook"
	"k8s.io/klog/v2"
)

func main() {
	defer klog.Flush()

	baseName := filepath.Base(os.Args[0])

	c, err := apphook.NewCommand(baseName)
	cmd.CheckError(err)
	cmd.CheckError(c.Execute())
}
