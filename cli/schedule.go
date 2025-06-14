// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cli

import (
	"context"
	"log"
	"time"

	"github.com/gorhill/cronexpr"
	"github.com/la5nta/pat/app"
)

type Job struct {
	expr *cronexpr.Expression
	cmd  string
	next time.Time
}

func scheduleLoop(ctx context.Context, a *app.App) {
	jobs := make([]*Job, 0, len(a.Config().Schedule))
	for exprStr, cmd := range a.Config().Schedule {
		expr := cronexpr.MustParse(exprStr)
		jobs = append(jobs, &Job{
			expr,
			cmd,
			expr.Next(time.Now()),
		})
	}

	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, j := range jobs {
					if time.Now().Before(j.next) {
						continue
					}
					log.Printf("Executing scheduled command '%s'...", j.cmd)
					execCmd(a, j.cmd)
					j.next = j.expr.Next(time.Now())
				}
			}
		}
	}()
}
