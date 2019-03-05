# cloudflare-kube-dns
Simple kubernetes Public DNS Updater

## Why not use external-dns?
I know.. I tried but it doesn't work for me, because I have dynamic public IP on the workers so I have a cname for every worker that is 
updated automatically.
So external-dns then try to use the CNAME to set the domains but you cannot asign multiple CNAME to a CNAME (Round robin) (not at least with cloudflare)

So I wrote a simple app that automatically detect changes on services(LoadBalancer,NodePort types) and ingress and update cloudflare.
This app only publish the public IP of nodes that have POD's running  looking through ingress/services.

This is useful if you are poor like me and try to build your own """load balancer""" using ingress and DNS roud-robin

##Install

```bash
#create secret with cloudflare config
kubectl create -n kube-system configmap cloudflare-external-dns \
               --from-literal=api=<CF_API_KEY> \
               --from-literal=mail=<CF_API_MAIL> \
               --from-literal=domain=<CF_API_DOMAIN>

#Install (NO RBAC)
kubectl apply -f https://raw.githubusercontent.com/segator/cloudflare-kube-dns/master/k8s.yaml
#Install (For RBAC)
kubectl apply -f https://raw.githubusercontent.com/segator/cloudflare-kube-dns/master/k8s-rbac.yaml

```

##Use
Before use take in mind this app will have 2 ways of take host Public IP.
* **Using Worker Name:** This application will try to resolve the worker name as simple domain
Name and then use the result IP's to create service/ingress domains.

* **Using node Annotation:** Add the annotation **auto-dns** to every node to force use this IP(you also can set a domain Name) 


OK, now we have the backend public IP of every worker defined, How Can I define then
new domains per service?

* **Ingress:** Create An Ingress and the system automatically will create the domain asociated to the pod's Ingress is redirecting traffic
* **Service:** Only nodePort and LoadBalancer service types are scanned, add **auto-dns:your.service.domain** as annotation of the service.

**Note:** This app only will asign IP's to domain from workers where have POD running.