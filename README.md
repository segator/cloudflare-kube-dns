# cloudflare-kube-dns
Simple kubernetes Public DNS Updater

## Why not use external-dns?
I know.. I tried but it doesn't work for me, because I have dynamic public IP on the workers so I have a cname for every worker that is 
updated automatically.
So external-dns then try to use the CNAME to set the domains but you cannot asign multiple CNAME to a CNAME (Round robin) (not at least with cloudflare)

So I wrote a simple app that automatically detect changes on services(LoadBalancer,NodePort types) and ingress and update cloudflare.
This app only publish the public IP of nodes that have POD's running  looking through ingress/services.

This is useful if you are poor like me and try to build your own """load balancer""" using ingress and DNS roud-robin

```bash
#create Config Map with cloudflare config

#Install
kubectl apply -f ...yml
```
