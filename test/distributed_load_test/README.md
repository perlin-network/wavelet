# Distributed Load Test on Kubernetes

```bash
cd test/distributed_load_test/k8s

kubectl create -f 01.namespace.yaml
kubectl create -f 02.deployment.coordinator.yaml
kubectl create -f 03.deployment.bootstrap.yaml
kubectl create -f 04.deployment.worker.yaml

# scaling the number of workers
NUM_REPLICAS=15
kubectl --namespace load-test scale deploy worker-deployment --replicas=$NUM_REPLICAS

kubectl --namespace load-test exec -it coordinator-deployment-xxx-yyy bash
curl http://localhost:8000/workers
curl -X POST http://localhost:8000/workers/init
curl -X POST http://localhost:8000/start
curl http://localhost:8000/summary
curl  -X POST http://localhost:8000/stop
```