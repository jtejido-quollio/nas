package controllers

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	cron "github.com/robfig/cron/v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ZSnapshotScheduleReconciler struct {
	client.Client
	Cfg Config
}

func (r *ZSnapshotScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.ZSnapshotSchedule
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	spec := obj.Spec
	ds := spec.DatasetName
	schedExpr := spec.Schedule
	prefix := spec.NamePrefix
	if strings.TrimSpace(prefix) == "" {
		prefix = "GMT"
	}
	format := spec.Format
	if strings.TrimSpace(format) == "" {
		format = "%Y.%m.%d-%H.%M.%S"
	}
	ret := spec.Retention

	na := NewNodeAgentClient(r.Cfg)

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	parsed, err := parser.Parse(strings.TrimSpace(schedExpr))
	if err != nil {
		obj.Status.Message = "invalid schedule"
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	now := time.Now().UTC()
	lastRunStr := obj.Status.LastRunTime
	var lastRun time.Time
	if lastRunStr != "" {
		// best-effort parse (RFC3339)
		t, _ := time.Parse(time.RFC3339, lastRunStr)
		lastRun = t
	}
	due := false
	if lastRunStr == "" {
		due = true
	} else {
		if !now.Before(parsed.Next(lastRun.UTC())) {
			due = true
		}
	}

	next := parsed.Next(now)
	obj.Status.NextRunTime = next.Format(time.RFC3339)

	if due {
		snapName := fmt.Sprintf("%s-%s", prefix, now.Format(strftimeToGo(format)))
		full := fmt.Sprintf("%s@%s", ds, snapName)
		body := map[string]any{"fullName": full, "recursive": spec.Recursive}
		var out any
		if err := na.do(ctx, "POST", "/v1/zfs/snapshot/create", body, &out, nil); err != nil {
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, &obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		obj.Status.LastRunTime = now.Format(time.RFC3339)
		obj.Status.LastSnapshotName = full

		// retention: keepLast (and keepHourly treated as extra keepLast for MVP)
		keepLast := int64(0)
		if ret != nil {
			if ret.KeepLast > keepLast {
				keepLast = ret.KeepLast
			}
			if ret.KeepHourly > keepLast {
				keepLast = ret.KeepHourly
			}
		}
		if keepLast > 0 {
			var list struct {
				OK    bool     `json:"ok"`
				Items []string `json:"items"`
			}
			q := make(url.Values)
			q.Set("dataset", ds)
			_ = na.do(ctx, "GET", "/v1/zfs/snapshot/list", nil, &list, q)
			managed := filterManaged(list.Items, ds, prefix)
			sort.Strings(managed)
			// newest last, so delete from beginning
			if int64(len(managed)) > keepLast {
				toDelete := managed[:int64(len(managed))-keepLast]
				for _, s := range toDelete {
					_ = na.do(ctx, "POST", "/v1/zfs/snapshot/destroy", map[string]any{"fullName": s}, &out, nil)
				}
			}
		}
	}

	obj.Status.Message = "OK"
	_ = r.Status().Update(ctx, &obj)

	wait := time.Until(next)
	if wait < 5*time.Second {
		wait = 5 * time.Second
	}
	if wait > 2*time.Minute {
		wait = 2 * time.Minute
	}
	return ctrl.Result{RequeueAfter: wait}, nil
}

func filterManaged(items []string, ds, prefix string) []string {
	var out []string
	for _, full := range items {
		parts := strings.Split(full, "@")
		if len(parts) != 2 {
			continue
		}
		if parts[0] != ds {
			continue
		}
		if strings.HasPrefix(parts[1], prefix+"-") {
			out = append(out, full)
		}
	}
	return out
}

func strftimeToGo(f string) string {
	out := f
	out = strings.ReplaceAll(out, "%Y", "2006")
	out = strings.ReplaceAll(out, "%m", "01")
	out = strings.ReplaceAll(out, "%d", "02")
	out = strings.ReplaceAll(out, "%H", "15")
	out = strings.ReplaceAll(out, "%M", "04")
	out = strings.ReplaceAll(out, "%S", "05")
	return out
}

func (r *ZSnapshotScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.ZSnapshotSchedule{}).
		Complete(r)
}
