// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"github.com/gorhill/cronexpr"
	"log"
	"time"
)

type Job struct {
	expr *cronexpr.Expression
	cmd  string
	next time.Time
}

func scheduleLoop() {
	jobs := make([]*Job, 0, len(config.Schedule))
	for exprStr, cmd := range config.Schedule {
		expr := cronexpr.MustParse(exprStr)
		jobs = append(jobs, &Job{
			expr,
			cmd,
			expr.Next(time.Now()),
		})
	}

	go func() {
		for {
			<-time.Tick(time.Second)
			for _, j := range jobs {
				if time.Now().Before(j.next) {
					continue
				}
				log.Printf("Executing scheduled command '%s'...", j.cmd)
				execCmd(j.cmd)
				j.next = j.expr.Next(time.Now())
			}
		}
	}()
}
