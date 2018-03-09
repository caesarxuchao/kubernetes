set -e

kubectl config use-context local
kubectl create -f ./webhook-secret.yaml || true
kubectl create -f ./webhook-deployment.yaml || true
kubectl create -f ./webhook-service.yaml || true
kubectl create -f ./webhook-register.yaml || true
sleep 1
