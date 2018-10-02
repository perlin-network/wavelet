#!/usr/bin/python
# -*- coding: utf-8 -*-

import argparse
from concurrent.futures import ThreadPoolExecutor
from requests_futures.sessions import FuturesSession
import json
import logging
import os
import sys
import traceback
import time
import datetime
from flask import Flask, request, jsonify

from endpoints import CoordinatorEndpoints, WorkerEndpoints
from utils import *

parser = argparse.ArgumentParser()
parser.add_argument(
    '--host',
    type=str,
    default="0.0.0.0",
    help="Host of the coordinator server (default: 0.0.0.0).")
parser.add_argument(
    '--port',
    type=int,
    default=8000,
    help="Port of the coordinator server (default: 8000).")

FORMAT = '[%(asctime)s] [%(levelname)s] [%(filename)s:%(lineno)d %(funcName)s()] - %(message)s'
logging.basicConfig(format=FORMAT)
logger = logging.getLogger(__name__)
logger.setLevel(logging.INFO)

WAVELET_PORT = 3000

WORKER_STATE_CREATED = 1
WORKER_STATE_INITED = 2
WORKER_STATE_STARTED = 3
WORKER_STATE_STOPPED = 4

class Worker(object):
    def __init__(self, name, host, port, wavelet_port):
        self.name = name
        self.host = host
        self.port = port
        self.wavelet_port = wavelet_port
        self.bootstrap = False
        self.state = WORKER_STATE_CREATED
        self.error_count = 0
        self.added = str(datetime.datetime.now())

    def __repr__(self):
        return str(self.__dict__)

class CoordinatorServer(object):
    def __init__(self, host, port):
        self.app = Flask(__name__)
        self.host = host
        self.port = port
        self.workers = {}
        self.register_routes()

    def serve(self):
        # in debug mode, the Flask app initializes twice
        if not self.app.debug or os.environ.get("WERKZEUG_RUN_MAIN") == "true":
            logger.info("starting coordinator on %s:%d" % (self.host, self.port))
            self.app.run(host=self.host, port=self.port, debug=False)

    def parse_summaries(self, summaries):
        summary_len = len(summaries)
        results = {
            "num_nodes": summary_len,
            "total_accepted_tx": 0,
            "total_uptime": 0,
            "total_laptime": 0,
            "total_consensus_duration": 0,
            "total_accept_by_tag_per_sec": dict(),
            "avg_accept_by_tag_per_sec_per_client": dict(),
        }
        for summary in summaries:
            results['total_accepted_tx'] += int(
                summary['num_accepted_tx'])
            results['total_uptime'] += float(summary['uptime'])
            results['total_laptime'] += float(summary['laptime'])
            results['total_consensus_duration'] += (
                int(summary['num_accepted_tx']) * float(summary['consensus_duration']))
            for key, val in summary['accept_by_tag_per_sec'].items():
                results['total_accept_by_tag_per_sec'][key] = results['total_accept_by_tag_per_sec'].get(key, 0) + int(val)
        if results['total_accepted_tx'] != 0:
            results['avg_consensus_duration'] = results['total_consensus_duration'] / results['total_accepted_tx']
        if summary_len != 0:
            results['avg_accepted_tx_per_client'] = results['total_accepted_tx'] / summary_len
            results['avg_accepted_tx_per_sec'] = results['total_accepted_tx'] / (results['total_laptime']/summary_len)
            results['avg_uptime'] = results['total_uptime'] / summary_len
            results['avg_laptime'] = results['total_laptime'] / summary_len
            for key, val in results["total_accept_by_tag_per_sec"].items():
                results['avg_accept_by_tag_per_sec_per_client'][key] = val / summary_len
        return results

    def get_bootstrap_workers(self):
        return [worker for worker in list(self.workers.values()) if worker.bootstrap == True]

    def register_routes(self):
        # example: curl http://localhost:8000/healthz
        @self.app.route(CoordinatorEndpoints.GetHealthz)
        def GetHealthz():
            msg = "OK"
            logger.info(msg)
            return return_message(msg)

        # example: curl http://localhost:8000/workers
        @self.app.route(CoordinatorEndpoints.GetWorkers)
        def GetWorkers():
            return json.dumps(self.workers, default=dumper, indent=2)

        # example: curl -d '{"name": "worker-1", "host":"localhost", "port":8000}' -H "Content-Type: application/json" -X POST http://localhost:8000/worker/add
        @self.app.route(CoordinatorEndpoints.PostAddWorker, methods=["POST"])
        def PostAddWorker():
            content = request.json
            check_for_fields(content, ["name", "host", "port", "wavelet_port"])
            name = content["name"]
            host = content["host"]
            port = content["port"]
            wavelet_port = content["wavelet_port"]
            if name not in self.workers:
                worker = Worker(name, host, port, wavelet_port)
                msg = "added new worker: %s" % worker
                if "bootstrap" in content and content["bootstrap"]:
                    worker.bootstrap = True
                    self.bootstrap = worker
                    msg = "added new bootstrap worker: %s" % worker
                self.workers[name] = worker
                logger.info(msg)
                return return_message(msg)
            else:
                worker = self.workers[name]
                worker.host = host
                worker.port = port
                worker.wavelet_port = wavelet_port
                self.workers[name] = worker
                msg = "updated worker: %s" % name
                logger.info(msg)
                return return_message(msg)

        # example: curl -X POST http://localhost:8000/workers/init
        @self.app.route(CoordinatorEndpoints.PostInitializeWorkers, methods=["POST"])
        def PostInitializeWorkers():
            content = request.json
            success = 0
            errors = 0
            skipped = 0

            bootstraps = self.get_bootstrap_workers()
            if len(bootstraps) == 0:
                msg = "cannot initialize, no bootstrap workers found."
                logger.error(msg)
                return InvalidUsage(msg)

            payload = {}
            logger.info("initializing %d bootstrap workers" % len(bootstraps))
            with FuturesSession(executor=ThreadPoolExecutor(max_workers=5)) as session:
                for worker in bootstraps:
                    if worker.state != WORKER_STATE_CREATED:
                        skipped += 1
                        continue
                    endpoint = "http://%s:%s%s" % (worker.host, worker.port, WorkerEndpoints.PostInitializeWorker)
                    future = session.post(endpoint, json=payload)
                    resp = future.result()
                    if not resp.ok:
                        errors += 1
                        logger.error(resp.content)
                        raise InvalidUsage(resp.content["message"])
                    success += 1
                    worker.state = WORKER_STATE_INITED

            # wait for bootstrap to initialize
            time.sleep(3)

            payload = {
                "peers": ",".join(["tcp://%s:%d" % (worker.host, worker.wavelet_port) for worker in bootstraps])
            }
            logger.info("initializing %d workers with payload: %s" % ((len(self.workers) - len(bootstraps)), payload))
            with FuturesSession(executor=ThreadPoolExecutor(max_workers=16)) as session:
                for worker in list(self.workers.values()):
                    if worker.bootstrap == True:
                        # skip the bootstrap since it already ran
                        continue
                    if worker.state != WORKER_STATE_CREATED:
                        skipped += 1
                        continue
                    endpoint = "http://%s:%s%s" % (worker.host, worker.port, WorkerEndpoints.PostInitializeWorker)
                    try:
                        future = session.post(endpoint, json=payload)
                        resp = future.result()
                        if resp.ok:
                            success += 1
                            worker.state = WORKER_STATE_INITED
                        else:
                            errors += 1
                    except:
                        errors += 1
            msg = "initialized %d workers, %d errors, %d skipped" % (success, errors, skipped)
            logger.info(msg)
            return return_message(msg)

        # example: curl -X POST http://localhost:8000/start
        @self.app.route(CoordinatorEndpoints.PostStartLoadTest, methods=["POST"])
        def PostStartLoadTest():
            payload = {}
            success = 0
            errors = 0
            skipped = 0
            session = FuturesSession(executor=ThreadPoolExecutor(max_workers=16))
            for worker in list(self.workers.values()):
                if worker.state != WORKER_STATE_INITED:
                    skipped += 1
                    continue
                endpoint = "http://%s:%s%s" % (worker.host, worker.port, WorkerEndpoints.PostStartLoadTest)
                future = session.post(endpoint, json=payload)
                resp = future.result()
                if resp.ok:
                    success += 1
                    worker.state = WORKER_STATE_STARTED
                else:
                    errors += 1
            msg = "started load test on %d clients, %d errors, %d skipped" % (success, errors, skipped)
            logger.info(msg)
            return return_message(msg)

        # example: curl -X POST http://localhost:8000/stop
        @self.app.route(CoordinatorEndpoints.PostStopLoadTest, methods=["POST"])
        def PostStopLoadTest():
            content = request.json
            payload = {}
            success = 0
            errors = 0
            skipped = 0
            session = FuturesSession(executor=ThreadPoolExecutor(max_workers=4))
            for worker in list(self.workers.values()):
                if worker.state != WORKER_STATE_STARTED:
                    skipped += 1
                    continue
                endpoint = "http://%s:%s%s" % (worker.host, worker.port, WorkerEndpoints.PostStopLoadTest)
                future = session.post(endpoint, json=payload)
                resp = future.result()
                if resp.ok:
                    success += 1
                    worker.state = WORKER_STATE_STOPPED
                else:
                    errors += 1
            msg = "stopped %d workers, %d errors, %d skipped" % (success, errors, skipped)
            logger.info(msg)
            return return_message(msg)

        # example: curl http://localhost:8000/summary
        @self.app.route(CoordinatorEndpoints.GetSummary)
        def GetSummary():
            def bg_cb(sess, resp):
                # parse the json storing the result on the response object
                if resp != None:
                    resp.data = resp.json()

            content = request.json
            summaries = []
            success = 0
            errors = 0
            skipped = 0
            session = FuturesSession(executor=ThreadPoolExecutor(max_workers=16))
            for worker in list(self.workers.values()):
                if worker.state != WORKER_STATE_STARTED:
                    skipped += 1
                    continue
                endpoint = "http://%s:%s%s" % (worker.host, worker.port, WorkerEndpoints.GetSummary)
                try:
                    future = session.get(endpoint, background_callback=bg_cb)
                    resp = future.result()
                    if resp.ok:
                        if resp.data.get("num_accepted_tx") != None:
                            success += 1
                            summaries.append(resp.data)
                            continue
                    errors += 1
                    worker.error_count += 1
                except:
                    errors += 1
                    worker.error_count += 1
                if worker.error_count > 1:
                    worker.state = WORKER_STATE_STOPPED
            logger.info("summary %d workers, %d errors, %d skipped" % (success, errors, skipped))
            parsed = self.parse_summaries(summaries)
            logger.info(parsed)
            return return_message(parsed)

        # example: curl -X POST http://localhost:8000/workers/delete
        @self.app.route(CoordinatorEndpoints.PostDeleteWorkers, methods=["POST"])
        def PostDeleteWorkers():
            msg = "deleted %d workers" % len(self.workers)
            self.workers = {}
            logger.info(msg)
            return return_message(msg)

        # example: curl -d '{"name": "worker-1"}' -H "Content-Type: application/json" -X POST http://localhost:8000/worker/delete
        @self.app.route(CoordinatorEndpoints.PostDeleteWorker, methods=["POST"])
        def DeleteWorker():
            content = request.json
            check_for_fields(content, ["name"])
            name = content["name"]
            if name not in self.workers:
                msg = "worker '%s' not found." % (name)
                logger.info(msg)
                return InvalidUsage(msg)
            else:
                del self.workers[name]
                msg = "deleted worker: %s" % name
                logger.info(msg)
                return return_message(msg)

        @self.app.errorhandler(InvalidUsage)
        def handle_invalid_usage(error):
            response = jsonify(error.to_dict())
            response.status_code = error.status_code
            return response

        @self.app.errorhandler(Exception)
        def handle_unexpected_error(error):
            etype, value, tb = sys.exc_info()
            logger.error(traceback.print_exception(etype, value, tb))
            response = jsonify(str(error))
            response.status_code = 500
            return response

########################################

def main(argv):
    args = parser.parse_args(argv)
    server = CoordinatorServer(args.host, args.port)
    server.serve()

if __name__ == '__main__':
    main(sys.argv[1:])
