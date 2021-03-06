/*
 * Copyright 2020 Google LLC.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Policy Generator automate generation of Google-recommended Policy Library constraints based on your Terraform configs.
//
// Usage:
// $ bazel run :policygen -- --output_dir=/tmp/constraints {--input_dir=/path/to/configs/dir|--input_plan=/path/to/plan/json|--input_state=/path/to/state/json}
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"flag"
	
	"github.com/GoogleCloudPlatform/healthcare/deploy/config"
	"github.com/GoogleCloudPlatform/healthcare/deploy/policygen/terraform"
	"github.com/GoogleCloudPlatform/healthcare/deploy/runner"
)

var (
	inputDir   = flag.String("input_dir", "", "Path to Terraform configs root directory. Cannot be specified together with other types of inputs.")
	inputPlan  = flag.String("input_plan", "", "Path to Terraform plan in json format, Cannot be specified together with other types of inputs.")
	inputState = flag.String("input_state", "", "Path to Terraform state in json format. Cannot be specified together with other types of inputs.")
	outputDir  = flag.String("output_dir", "", "Path to directory to write generated policies")
)

func main() {
	flag.Parse()

	isOneNonEmpty := func(ss ...string) bool {
		var n int
		for _, s := range ss {
			if s != "" {
				n++
			}
		}
		return n == 1
	}

	if !isOneNonEmpty(*inputDir, *inputPlan, *inputState) {
		log.Fatal("exactly one of --input_dir, --input_plan or --input_state must be specified")
	}

	if *outputDir == "" {
		log.Fatal("--output_dir must be set")
	}

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {

	var err error
	*outputDir, err = config.NormalizePath(*outputDir)
	if err != nil {
		return fmt.Errorf("normalize path %q: %v", *outputDir, err)
	}

	var resources []terraform.Resource

	switch {
	case *inputDir != "":
		if resources, err = resourcesFromDir(*inputDir); err != nil {
			return fmt.Errorf("read resources from configs directory: %v", err)

		}
	case *inputPlan != "":
		if resources, err = resourcesFromPlan(*inputPlan); err != nil {
			return fmt.Errorf("read resources from plan: %v", err)
		}
	case *inputState != "":
		if resources, err = resourcesFromState(*inputState); err != nil {
			return fmt.Errorf("read resources from state: %v", err)
		}
	}

	for _, r := range resources {
		log.Printf("%+v", r)
	}

	// TODO: generate policies that do not rely on input config.
	// TODO: generate policies that rely on input config.

	return nil
}

func resourcesFromDir(path string) ([]terraform.Resource, error) {
	path, err := config.NormalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("normalize path %q: %v", path, err)
	}

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	rn := &runner.Default{}
	tfCmd := func(args ...string) error {
		cmd := exec.Command("terraform", args...)
		cmd.Dir = tmpDir
		return rn.CmdRun(cmd)
	}
	tfCmdOutput := func(args ...string) ([]byte, error) {
		cmd := exec.Command("terraform", args...)
		cmd.Dir = tmpDir
		return rn.CmdOutput(cmd)
	}

	fs, err := filepath.Glob(filepath.Join(path, "*"))
	if err != nil {
		return nil, err
	}
	cp := exec.Command("cp", append([]string{"-a", "-t", tmpDir}, fs...)...)
	if err := rn.CmdRun(cp); err != nil {
		return nil, fmt.Errorf("copy configs to temp directory: %v", err)
	}

	if err := tfCmd("init"); err != nil {
		return nil, fmt.Errorf("terraform init: %v", err)
	}

	planPath := filepath.Join(tmpDir, "plan.tfplan")
	if err := tfCmd("plan", "-out", planPath); err != nil {
		return nil, fmt.Errorf("terraform plan: %v", err)
	}

	b, err := tfCmdOutput("show", "-json", planPath)
	if err != nil {
		return nil, fmt.Errorf("terraform show: %v", err)
	}

	resources, err := terraform.ReadPlanResources(b)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json: %v", err)
	}
	return resources, nil
}

func resourcesFromPlan(path string) ([]terraform.Resource, error) {
	path, err := config.NormalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("normalize path %q: %v", path, err)
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read json: %v", err)
	}
	resources, err := terraform.ReadPlanResources(b)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json: %v", err)
	}
	return resources, nil
}

func resourcesFromState(path string) ([]terraform.Resource, error) {
	path, err := config.NormalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("normalize path %q: %v", path, err)
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read json: %v", err)
	}
	resources, err := terraform.ReadStateResources(b)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json: %v", err)
	}
	return resources, nil
}
