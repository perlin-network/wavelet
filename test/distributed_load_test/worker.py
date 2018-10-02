#!/usr/bin/python
# -*- coding: utf-8 -*-

from flask import Flask, request, jsonify
import argparse
from concurrent import futures
from concurrent.futures import ProcessPoolExecutor as Pool
import logging
import json
import os
import random
import requests
import socket
import subprocess
import sys
import threading
import time
import toml
import tempfile

from endpoints import WorkerEndpoints, CoordinatorEndpoints
import utils

parser = argparse.ArgumentParser(
    description='Load test worker')
parser.add_argument(
    "--name",
    type=str,
    help='worker name.')
parser.add_argument(
    "--host",
    type=str,
    required=False,
    default=socket.gethostbyname(socket.gethostname()),
    help='host of the worker server.')
parser.add_argument(
    "--port",
    type=int,
    default=8100,
    help='port of the worker server (default: 8100).')
parser.add_argument(
    "--bootstrap",
    type=lambda x: (str(x).lower() in ("yes", "true", "t", "1")),
    default=False,
    help='this worker is a bootstrap node (default: False).')
parser.add_argument(
     "--coordinator_host",
    type=str,
    default="",
    help='coordinator server host.')
parser.add_argument(
    "--coordinator_port",
    type=int,
    default=8000,
    help='coordinator server port (default: 8000).')
parser.add_argument(
    "--wctl_exec",
    type=str,
    default="./wctl",
    help='path to the wctl executable')
parser.add_argument(
    "--main_exec",
    type=str,
    default="./wavelet",
    help='path to the main executable')
parser.add_argument(
    "--wavelet_port",
    type=int,
    default=3000,
    help='port for the wavelet executable (default: 3000).')
parser.add_argument(
    "--api_port",
    type=int,
    default=3100,
    help='port for the wavelet api (default: 3100).')
parser.add_argument(
    "--peers",
    type=str,
    required=False,
    help='peers argument for the main executable')
parser.add_argument(
    "--pk_file",
    type=str,
    default="test/keypairs.csv",
    help='path to the private/public key file.')
parser.add_argument(
    "--pk_index",
    type=int,
    required=False,
    help='index in the private/public key file that will be sending transactions (defaults to random).')
parser.add_argument(
    "--sleep_interval",
    type=float,
    default=0.0,
    help='broadcast sleep interval (default: 1s).')
parser.add_argument(
    "--stats_poll_interval",
    type=float,
    default=0.0,
    help='seconds to poll system stats (default: off).')

FORMAT = '[%(asctime)s] [%(levelname)s] [%(filename)s:%(lineno)d %(funcName)s()] - %(message)s'
logging.basicConfig(format=FORMAT)
logger = logging.getLogger(__name__)
logger.setLevel(logging.INFO)

GENESIS_BALANCE = 1000000

#########################

class LoadTestWorker(object):
    def __init__(self, args):
        self.app = Flask(__name__)
        self.host = args.host
        if self.host is None:
            try:
                self.host = socket.gethostbyname(socket.gethostname())
            except:
                self.host = socket.gethostbyname("")
        self.wavelet_port = args.wavelet_port
        self.api_port = args.api_port
        self.name = args.name
        self.port = args.port
        self.wctl_exec = args.wctl_exec
        self.bootstrap = args.bootstrap
        if self.name is None:
            self.name = "%s:%s" % (self.host, self.port)
        self.reg_worker = RegisterWorker({
            "coordinator_host": args.coordinator_host,
            "coordinator_port": args.coordinator_port,
            "name": self.name,
            "host": self.host,
            "port": self.port,
            "wavelet_port": self.wavelet_port,
            "bootstrap": self.bootstrap,
        })
        self.main_runner = MainRunner({
            "pk_file": args.pk_file,
            "pk_index": args.pk_index,
            "peers": args.peers,
            "wctl_exec": args.wctl_exec,
            "host": args.host,
            "wavelet_port": args.wavelet_port,
            "sleep_interval": args.sleep_interval,
            "main_exec": args.main_exec,
            "api_port": self.api_port,
            "stats_poll_interval": args.stats_poll_interval,
        })
        self.register_routes()

    def serve(self):
        logger.info("starting worker on port %d" % self.port)
        self.reg_worker.start()
        self.app.run(host='0.0.0.0', port=self.port, debug=False)

    def register_routes(self):
        # example: curl http://localhost:8000/healthz
        @self.app.route(WorkerEndpoints.GetHealthz)
        def GetHealthz():
            logger.info("healthz")
            return utils.return_message("ok")

        # example: curl -d '{"peers": "tcp://localhost:3000"}' -H "Content-Type: application/json" -X POST http://localhost:8000/worker/init
        @self.app.route(WorkerEndpoints.PostInitializeWorker, methods=["POST"])
        def PostInitializeWorker():
            logger.info("Init")
            content = request.json
            if content is not None and "peers" in content:
                self.main_runner.peers = content["peers"]
                logger.info("got peers: %s", self.main_runner.peers)
            # reset the main runner
            self.main_runner.init_thread()
            self.main_runner.start_main()
            return utils.return_message("started main")

        # example: curl -X POST http://localhost:8000/start
        @self.app.route(WorkerEndpoints.PostStartLoadTest, methods=["POST"])
        def PostStartLoadTest():
            logger.info("LoadTestStart")
            self.main_runner.thread.stop_thread = False
            self.main_runner.thread.start()
            self.main_runner.stats_thread.start()
            return utils.return_message("ok")

        # example: curl -X POST http://localhost:8000/stop
        @self.app.route(WorkerEndpoints.PostStopLoadTest, methods=["POST"])
        def PostStopLoadTest():
            logger.info("LoadTestStop")
            self.main_runner.thread.stop_thread = True
            return utils.return_message("ok")

        # example: curl http://localhost:8000/summary
        @self.app.route(WorkerEndpoints.GetSummary)
        def GetSummary():
            logger.info("Summary:")
            if self.main_runner.is_running() == False:
                return utils.return_message("main not running")

            return_code, results = self.get_summary()
            if return_code != 0:
                msg = "got error (%d) running the last command: %s" % (return_code, results)
                logger.error(msg)
                return utils.return_message(msg)

            summary = parse_stats(results)
            logger.info(json.dumps(summary))
            return jsonify(summary)

    def get_summary(self):
        # Example: wctl -privkey abcd -remote localhost:3900 stats_summary
        pargs = [
            self.wctl_exec,
            "-remote", "localhost:%d" % self.api_port,
            "-privkey", self.main_runner.private_key,
            "stats_summary",
        ]
        return exec_process(pargs)

#########################


class MainRunner(object):
    def __init__(self, args):
        self.pk_file = args["pk_file"]
        self.pk_index = args["pk_index"]
        self.peers = args["peers"]
        self.wctl_exec = args["wctl_exec"]
        self.host = args["host"]
        self.wavelet_port = args["wavelet_port"]
        self.sleep_interval = args["sleep_interval"]
        self.main_exec = args["main_exec"]
        self.api_port = args["api_port"]
        self.stats_thread = StatsPoller(self.api_port, self, args["stats_poll_interval"])

        # pick a random key from the keypairs.csv
        self.keys, self.key_idx = get_keys(self.pk_file, self.pk_index)

        # these are the keys being used
        self.public_key = self.keys[self.key_idx][1]
        self.private_key = self.keys[self.key_idx][0]

        # generate a genesis file at genesis_file.name
        self.genesis_file = create_genesis(self.keys)

        self.init_thread()

    def init_thread(self):
        # setup the broadcast loop
        self.thread = BroadcastTransaction(
            self.wctl_exec,
            self.private_key,
            self.keys,
            self.key_idx,
            self.api_port,
            self.sleep_interval)

    def is_running(self):
        return not self.thread.stop_thread

    def start_main(self):
        logger.info("starting main process %s" % (self.main_exec))

        # generate a local_config.json file to allow everything to run
        local_config_file = create_local_config(
            self.public_key,
            self.private_key,
            self.peers,
            self.wavelet_port,
            self.api_port)

        # spawn the main process
        spawn_main_process(self.main_exec, self.host,
                           local_config_file, self.genesis_file)

class RegisterWorker(threading.Thread):
    def __init__(self, args):
        threading.Thread.__init__(self)
        self.coordinator_host = args["coordinator_host"]
        self.coordinator_port = args["coordinator_port"]
        self.name = args["name"]
        self.host = args["host"]
        self.port = args["port"]
        self.wavelet_port = args["wavelet_port"]
        self.bootstrap = args["bootstrap"]

    def run(self):
        if self.coordinator_host == None or len(self.coordinator_host) == 0:
            logger.info("skipped registered worker with coordinator, parameter not provided")
            return

        logger.info("beginning to register worker with coordinator at %s:%s" % (self.coordinator_host, self.coordinator_port))
        payload = {
            "name": self.name,
            "host": self.host,
            "port": self.port,
            "wavelet_port": self.wavelet_port,
            "bootstrap": self.bootstrap,
        }
        endpoint = "http://%s:%s%s" % (self.coordinator_host, self.coordinator_port, CoordinatorEndpoints.PostAddWorker)
        resp = requests.post(endpoint, json=payload)
        if not resp.ok:
            logger.fatal(resp.content)
            exit(1)
        logger.info("successfully registered worker with coordinator at %s:%s" % (self.coordinator_host, self.coordinator_port))
        return

class StatsPoller(threading.Thread):
    def __init__(self, api_port, runner, interval):
        threading.Thread.__init__(self)
        self.debug_port = api_port
        self.runner = runner
        self.interval = interval

    def run(self):
        if self.interval <= 0.0001:
            return

        endpoint = "http://localhost:%s/debug/vars" % (self.debug_port)
        while True:
            if self.runner.thread.stop_thread:
                break

            resp = requests.get(endpoint)
            logger.info(resp.json())
            time.sleep(self.interval)


class BroadcastTransaction(threading.Thread):
    def __init__(self, wctl_exec, private_key, keys, key_idx, api_port, sleep_interval):
        threading.Thread.__init__(self)
        self.stop_thread = True
        self.wctl_exec = wctl_exec
        self.private_key = private_key
        self.keys = keys
        self.num_keys = len(keys)
        self.key_idx = key_idx
        self.sleep_interval = sleep_interval
        self.api_port = api_port

    def run(self):
        # first reset the stats
        pargs = [
                self.wctl_exec,
                "-remote", "localhost:%d" % self.api_port,
                "-privkey", self.private_key,
                "stats_reset"
        ]
        return_code, stdout = exec_process(pargs)
        if return_code != 0:
            logger.error("got error (%d) running the last command: %s" % (return_code, stdout))
            self.stop_thread = True
            return

        consecutive_errors = 0
        time.sleep(1.0)
        # example: wctl -privkey abcd -remote localhost:3900 pay 8f9b4ae0364280e6a0b988c149f65d1badaeefed2db582266494dd79aa7c821a 10
        while True:
            if self.stop_thread:
                break

            try:
                recipientIdx = random.randint(0, self.num_keys)
                while recipientIdx == self.key_idx:
                    # if it is the key_idx, which it itself, pick a new index
                    recipientIdx = random.randint(0, self.num_keys)
                recipient = self.keys[recipientIdx][1]
                amount = random.randint(1, 5)
                pargs = [
                    self.wctl_exec,
                    "-remote", "localhost:%d" % self.api_port,
                    "-privkey", self.private_key,
                    "pay",
                    recipient,
                    amount
                ]
                return_code, stdout = exec_process(pargs)
                if return_code != 0:
                    consecutive_errors += 1
                    if consecutive_errors > 10:
                        logger.error("got error (%d) running the last command: %s" % (return_code, stdout))
                        self.stop_thread = True
                        return
                else:
                    consecutive_errors = 0
            except:
                logger.info("Unexpected error: %s" % sys.exc_info()[0])

            time.sleep(self.sleep_interval)
        return

def get_keys(keyFile, argIdx):
    # pick a random key from the keypairs.csv
    with open(keyFile) as f:
        keys = f.readlines()
    keys = [x.strip().split(",") for x in keys]
    numKeys = len(keys)
    # start off random and then use the arg if it's there
    keyIdx = random.randint(0, numKeys - 1)
    if argIdx != None and argIdx >= 0 and argIdx < numKeys:
        keyIdx = argIdx
    return keys, keyIdx


def create_genesis(keys):
    fd, path = tempfile.mkstemp()
    with os.fdopen(fd, 'w') as tmp:
        for _, publicKey in keys:
            tmp.write("{},{}\n".format(publicKey, GENESIS_BALANCE))
    return path


def create_local_config(publicKey, privateKey, peers, port, api_port):
    # see the config.0.toml as an example
    local_config = dict()
    local_config["port"] = int(port)
    if peers != None and len(peers) > 0:
        local_config["peers"] = peers.split(",")
    local_config["privkey"] = privateKey
    local_config["api"] = dict()
    local_config["api"]["port"] = "0.0.0.0:%d" % api_port
    local_config["api.clients"]["public_key"] = []
    local_config["api.clients"]["_private_key"] = []
    local_config["api.clients"]["public_key"].append(publicKey)
    local_config["api.clients"]["_private_key"].append(privateKey)
    fd, path = tempfile.mkstemp()
    with os.fdopen(fd, 'w') as tmp:
        tmp.write(toml.dumps(local_config))
    return path


def parse_stats(lines):
    # 2018/08/14 14:02:14 SessionToken = 03d04594-f3a4-4000-95ec-50b29085e3ce
    # 2018/08/14 14:02:14 {"wavelet_consensus_duration":0.6343094,"wavelet_num_accepted_transactions":117,"wavelet_num_accepted_transactions_per_sec":0}
    # --> return json.loads({"wavelet_consensus_duration":0.6343094,"wavelet_num_accepted_transactions":117,"wavelet_num_accepted_transactions_per_sec":0})
    last_line = lines[-1].strip()
    the_json = last_line.split(" ", 2)  # split that log twice
    json_summary = json.loads(the_json[-1])
    return json_summary


def spawn_main_process(main_exec, exec_host, local_config_file, genesis_file):
    pargs = [
        main_exec,
        "-host", exec_host,
        "-config", local_config_file,
        "-genesis", genesis_file,
    ]
    return exec_background_process(pargs)

def exec_background_process(pargs):
    logger.info("exec_background_process: %s" % pargs)
    subprocess.Popen(pargs, close_fds=True)


def exec_process(pargs):
    logger.info("exec_process: %s" % pargs)
    proc = subprocess.Popen(
        pargs, shell=False, stdout=subprocess.PIPE, universal_newlines=True)

    # Wait until process terminates (without using p.wait())
    poll_count = 0
    while proc.poll() is None:
        if poll_count >= 20:
            return -1, "timed out after 10s"
        poll_count += 1

        # Process hasn't exited yet, let's wait some
        time.sleep(0.5)

    return proc.returncode, proc.stdout.readlines()

def exec_process_pool(pargs):
    logger.info("exec_process: %s" % pargs)
    parallelism = 4
    with Pool(max_workers=parallelism) as executor:
        for i in range(parallelism):
            future = executor.submit(subprocess.Popen, pargs, shell=False, stdout=subprocess.PIPE, universal_newlines=True)

########################################

def main(argv):
    args = parser.parse_args(argv)
    worker = LoadTestWorker(args)
    worker.serve()

if __name__ == '__main__':
    main(sys.argv[1:])