package controllers

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

type Config struct {
	NodeAgentBaseURL string
	AuthHeader       string
	AuthValue        string
	Namespace        string
}

func SetupAll(mgr ctrl.Manager, cfg Config) error {
	if err := (&ZPoolReconciler{Client: mgr.GetClient(), Cfg: cfg}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err := (&ZDatasetReconciler{Client: mgr.GetClient(), Cfg: cfg}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err := (&ZSnapshotReconciler{Client: mgr.GetClient(), Cfg: cfg}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err := (&NASShareReconciler{Client: mgr.GetClient(), Cfg: cfg}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err := (&ZSnapshotScheduleReconciler{Client: mgr.GetClient(), Cfg: cfg}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err := (&ZSnapshotRestoreReconciler{Client: mgr.GetClient(), Cfg: cfg}).SetupWithManager(mgr); err != nil {
		return err
	}
	return nil
}
