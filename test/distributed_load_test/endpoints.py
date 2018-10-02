#!/usr/bin/python
# -*- coding: utf-8 -*-

class CoordinatorEndpoints:
    GetHealthz = "/healthz"
    PostStartLoadTest = "/start"
    PostStopLoadTest = "/stop"
    GetSummary = "/summary"
    GetWorkers = "/workers"
    PostDeleteWorkers = "/workers/delete"
    PostInitializeWorkers = "/workers/init"
    PostAddWorker = "/worker/add"
    PostDeleteWorker = "/worker/delete"

class WorkerEndpoints:
    GetHealthz = "/healthz"
    PostStartLoadTest = "/start"
    PostStopLoadTest = "/stop"
    GetSummary = "/summary"
    PostInitializeWorker = "/worker/init"
