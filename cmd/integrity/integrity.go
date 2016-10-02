// Integrity executes integrity tests against resources
// on multiple services.
//
// Theory of operation:
//  - each service exposes an endpoint to run diagnostic
//    tests.
//
//  - tests check the integrity of a resource on the
//    service. A major assumption is that each resource
//    has a singular, unique id given a url/path and
//    that ids are consistent across services.
//
package main

import (
	"fmt"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"time"
	"github.com/robfig/cron"
)

// TestCase is a pipeline data structure.
type TestCase struct {
	TaskID int
	Name string
	Path string
	Target string
	Callback chan Result
}

// Task records the parameters of a test run.
type Task struct {
	// resource identifier to find this task again.
	TaskID int `json:"-"`
	// defines a list of resources to target
	Targets []string
	// defines a list of tests to run by path
	Tests   []struct{
		// name unifies lets you compare results across tasks and targets, even if path changes
		Name string
		// path is a URL with %s that takes a target and gets back results
		Path string
	}
}

// Result records the result of a single test run for a task.
type Result struct {
	// taskID this result was retrieved for.
	TaskID int
	// test name - the task can be used to recover the path
	Name string
	// target name - used with path, can reconstruct URL.
	Target string
	// time when result retrieved.
	RunTime time.Time
	// result value
	Result bool
	// human-readable explanation.
	Note string
}

func main() {
	fmt.Println("Act with integrity.")


	runner := make(chan TestCase)
	// number of retrieval request workers.
	for w := 0; w < 5; w++ {
		Retrieve(runner)
	}

	c := cron.New()
	c.AddFunc("@every 10s", taskJob(1, "test1.json", runner))
	c.AddFunc("@every 10s", taskJob(2, "test2.json", runner))
	c.Start()

	// wait forever and let cron do its thing- may
	// be replaced with an http handler eventually.
	for {

	}
}

// taskJob represents a diagnostic testing task that can be
// scheduled.
func taskJob(id int, file string, runner chan TestCase) cron.FuncJob {
	return func() {
		// TODO in memory storage

		fmt.Printf("Running %d\n", id)
		dat, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}
		var p Task
		json.Unmarshal(dat, &p)

		// outer product of targets and tests.
		p.TaskID = id

		callback := make(chan Result)

		expected := len(p.Targets) * len(p.Tests)
		go func() {
			j := 0
			results := make([]Result, 0)
			for {
				j++
				i, more := <-callback
				if more {
					// Collect results
					results = append(results, i)
				}
				if !more || j >= expected {
					// Write out results
					fmt.Printf("All done with %d:\n", id)
					for _, k := range results {
						fmt.Printf("  #%d %s @ %s \n    %v -> %s\n",  k.TaskID, k.Name, k.RunTime.Format(time.RFC3339), k.Result, k.Note)
					}
					close(callback)
					return
				}
			}
		}()

		// sends outer product of targets and test to http queue.
		for _, tgt := range p.Targets {
			for _, t := range p.Tests {
				runner <- TestCase{
					Path: t.Path,
					Name: t.Name,
					Target: tgt,
					TaskID: p.TaskID,
					Callback: callback,
				}
			}
		}
	}
}

func Retrieve(in <-chan TestCase) {
	go func() {
		for i := range in {
			client := http.DefaultClient

			resp, err := client.Get(fmt.Sprintf(i.Path, i.Target))
			if err != nil {
				panic(err)
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				panic(err)
			}

			var r struct {
				Result bool
				Note string
			}
			err = json.Unmarshal(body, &r)
			if err != nil {
				panic(err)
			}

			tr := Result{}
			tr.TaskID = i.TaskID
			tr.Name = i.Name
			tr.Target = i.Target
			tr.Result = r.Result
			tr.Note = r.Note
			tr.RunTime = time.Now()
			i.Callback <- tr
		}
	}()
}