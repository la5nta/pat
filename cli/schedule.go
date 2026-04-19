// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cli

import (
	"context"
	"log"
	"time"

	"github.com/adhocore/gronx"
	"github.com/la5nta/pat/app"
)

type Job struct {
	expr string
	cmd  string
	next time.Time
}

func scheduleLoop(ctx context.Context, a *app.App) {
	jobs := make([]*Job, 0, len(a.Config().Schedule))
	for exprStr, cmd := range a.Config().Schedule {
		if !gronx.IsValid(exprStr) {
			log.Printf("Skipping invalid schedule expression %q", exprStr)
			continue
		}
		next, err := gronx.NextTickAfter(exprStr, time.Now(), false)
		if err != nil {
			log.Printf("Skipping invalid schedule expression %q: %v", exprStr, err)
			continue
		}
		jobs = append(jobs, &Job{exprStr, cmd, next})
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
					j.next, _ = gronx.NextTickAfter(j.expr, time.Now(), false)
				}
			}
		}
	}()
}
