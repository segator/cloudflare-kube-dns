package main

import (
	"flag"
	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)
type DomainNode struct {
	domainName string
	nodes  []string
	proxy bool
	ips []net.IP
}

type domainNodeList struct {
	items []*DomainNode
}

type CloudFlareActions interface {
	getZoneID() (string,error)

}
type CloudFlare struct {
	api cloudflare.API
	baseDomain string
}

func (c *CloudFlare) getZoneID() (string, error) {
	zoneIDInterface,err :=Do(func()  (interface{},*RetryError){
		id, err := c.api.ZoneIDByName(c.baseDomain)
		if err!=nil {
			return "",&RetryError{true,err	}
		}
		return id,nil
	})
	if err!=nil {
		return "",err
	}
	return zoneIDInterface.(string),nil
}

func (c *CloudFlare) getDNSRecord(zoneID string, filterRecord cloudflare.DNSRecord) ([]cloudflare.DNSRecord, error) {
	records,err :=Do(func()  (interface{},*RetryError){
		records, err := c.api.DNSRecords(zoneID,filterRecord)
		if err!=nil {
			return nil,&RetryError{true,err	}
		}
		return records,nil
	})
	if err!=nil {
		return nil,err
	}
	return records.([]cloudflare.DNSRecord),nil
}

func (c *CloudFlare) deleteDNSRecord(zoneID string, recordID string) error {
	_,err :=Do(func()  (interface{},*RetryError){
		err := c.api.DeleteDNSRecord(zoneID,recordID)
		if err!=nil {
			return nil,&RetryError{true,err	}
		}
		return nil,nil
	})
	return err
}

func (c *CloudFlare) createDNSRecord(zoneID string, record cloudflare.DNSRecord) (*cloudflare.DNSRecordResponse, error) {
	response,err :=Do(func()  (interface{},*RetryError){
		response,err := c.api.CreateDNSRecord(zoneID,record)
		if err!=nil {
			return nil,&RetryError{true,err	}
		}
		return response,nil
	})
	if err!=nil {
		return nil,err
	}
	return response.(*cloudflare.DNSRecordResponse),nil


}


func main() {


	var cfApiKey string
	var cfApiEmail string
	var cfDomain string
	flag.StringVar(&cfApiKey,"cloudflare-key","","Cloudflare API Key")
	flag.StringVar(&cfApiEmail,"cloudflare-mail","","Cloudflare API Email")
	flag.StringVar(&cfDomain,"cloudflare-domain","","Cloudflare Zone Domain")
	if cfApiKey =="" {
		cfApiKey=os.Getenv("CF_API_KEY")
	}
	if cfApiEmail =="" {
		cfApiEmail=os.Getenv("CF_API_MAIL")
	}

	if cfDomain == "" {
		cfDomain=os.Getenv("CF_API_DOMAIN")
	}






	var kubeconfig string
	var masterURL string
	home := homeDir();
	defaultConfigPath := filepath.Join(home, ".kube", "config")
	_, err := os.Stat(defaultConfigPath);
	if home=="" || os.IsNotExist(err) {
		flag.StringVar(&kubeconfig,"kubeconfig", "", "absolute path to the kubeconfig file")
	}else{
		flag.StringVar(&kubeconfig,"kubeconfig", defaultConfigPath, "(optional) absolute path to the kubeconfig file")
	}

	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.Parse()
	chanOSSignal := make(chan os.Signal)
	signal.Notify(chanOSSignal, os.Interrupt, syscall.SIGTERM)

	cloudflareApi, err := cloudflare.New(cfApiKey, cfApiEmail)
	if err!=nil {
		panic(err)
	}
	cloudflareExecutor:=CloudFlare{
		api: *cloudflareApi,
		baseDomain: cfDomain,
	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	loop:=true
	for loop {
		zoneID,err := cloudflareExecutor.getZoneID()
		if err!=nil {
			panic(err.Error())
		}

		dnss,err:=cloudflareExecutor.getDNSRecord(zoneID,cloudflare.DNSRecord{})
		if err!=nil {
			panic(err.Error())
		}
		cloudFlareDNSRecordsFromCloudflare:= getOnlyTypes(dnss,"A")
		cloudFlareDNSRecordsFromCloudflare = getOnlyControlledRecords(cloudFlareDNSRecordsFromCloudflare,getOnlyTypes(dnss,"TXT"))


		domainsNodeList := &domainNodeList{
		}
		getValidServices(clientset,domainsNodeList)
		getValidIngress(clientset,domainsNodeList)
		var cloudFlareDNSRecordsFromKube []*cloudflare.DNSRecord
		for _,domainNode := range domainsNodeList.items {
			for _,server := range domainNode.nodes {
				ipsLookup, _ := net.LookupIP(server)
				for _, ipLookup := range ipsLookup {
					dnsRecord := &cloudflare.DNSRecord{
						Name:domainNode.domainName,
						Content:ipLookup.String(),
						Type:"A",
						TTL:1,
						Proxied:false,
					}
					cloudFlareDNSRecordsFromKube = append(cloudFlareDNSRecordsFromKube,dnsRecord)
				}
			}
		}
		//Find records to be deleted
		for _, cfDNSRecord := range cloudFlareDNSRecordsFromCloudflare {
			existOnKube:=false
			for _, kubeDNSRecord := range cloudFlareDNSRecordsFromKube {
				if strings.HasSuffix(kubeDNSRecord.Name,cfDomain) {
					if DNSRecordEqual(kubeDNSRecord,cfDNSRecord) {
						existOnKube=true
						break
					}
				}
			}
			if !existOnKube{
				deleteDNSRecord(&cloudflareExecutor,zoneID,cfDNSRecord)
			}
		}

		//Find records to be inserted
		for _, kubeDNSRecord := range cloudFlareDNSRecordsFromKube {
			if strings.HasSuffix(kubeDNSRecord.Name,cfDomain) {
				existOnCF := false
				for _, cfDNSRecord := range cloudFlareDNSRecordsFromCloudflare {
					if DNSRecordEqual(kubeDNSRecord, cfDNSRecord) {
						existOnCF = true
						break
					}
				}
				if !existOnCF {
					createDNSRecord(&cloudflareExecutor, zoneID, *kubeDNSRecord)
				}
			}
		}


		select {
			case <-chanOSSignal:
				log.Println("Signal detected, exitting...")
				loop=false
			case <-time.After(10 * time.Second):
		}

	}
}

func deleteDNSRecord(cloudflareApi *CloudFlare, zoneID string, record cloudflare.DNSRecord) {
	log.Println("Deleting DNS: " + record.Name + " " + record.Type + " " +record.Content)
	err := cloudflareApi.deleteDNSRecord(zoneID,record.ID)

	if err!=nil {
		panic(err)
	}
}

func createDNSRecord(cloudflareApi *CloudFlare,zoneID string, record cloudflare.DNSRecord) {
	log.Println("Creating DNS: " + record.Name + " " + record.Type + " " +record.Content)
	response,err := cloudflareApi.createDNSRecord(zoneID,record)
	if err!=nil {
		panic(err)
	}
	if !response.Success {
		panic("error on create DNS:"+record.Name)
	}
	record.Type="TXT"
	record.Content="auto-dns"
	cloudflareApi.api.CreateDNSRecord(zoneID,record)
}

func DNSRecordEqual(record *cloudflare.DNSRecord, record2 cloudflare.DNSRecord) bool {
	return record.Content == record2.Content && record.Type == record2.Type && record.Name== record2.Name && record.Proxied == record2.Proxied
}

func getOnlyControlledRecords(records []cloudflare.DNSRecord, txtRecords []cloudflare.DNSRecord) []cloudflare.DNSRecord {
	var filteredRecords []cloudflare.DNSRecord
	for _, record := range records {
		for _, txtRecord := range txtRecords {
			if record.Name == txtRecord.Name && txtRecord.Content == "auto-dns" {
				filteredRecords = append(filteredRecords,record)
			}
		}
	}
	return filteredRecords
}

func getOnlyTypes(records []cloudflare.DNSRecord, recordType string) []cloudflare.DNSRecord {
	var filteredRecords []cloudflare.DNSRecord
	for _, dns := range records {
		if dns.Type == recordType {
			filteredRecords = append(filteredRecords,dns)
		}
	}
	return filteredRecords
}



func getValidIngress(clientset *kubernetes.Clientset, domainsNodeList *domainNodeList) {
	ingresses, err := clientset.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	var services []corev1.Service
	for _, ingress := range ingresses.Items {
		for _, ingressRule := range ingress.Spec.Rules {
			domain := ingressRule.Host
			for _ , ingressRulePath := range ingressRule.HTTP.Paths {
				service,err := clientset.CoreV1().Services(ingress.Namespace).Get(ingressRulePath.Backend.ServiceName,metav1.GetOptions{})
				if err != nil {
					panic(err.Error())
				}
				if service.Annotations == nil {
					service.Annotations = make(map[string]string)
				}
				service.Annotations["auto-dns"] = domain
				service.Annotations["auto-dns-proxy"] = ingress.Annotations["auto-dns-proxy"]
				services = append(services,*service)
			}
		}
	}
	filterByServiceList(clientset,services,domainsNodeList)
}

func getValidServices(clientset *kubernetes.Clientset, domainsNodeList *domainNodeList) {
	services, err := clientset.CoreV1().Services("").List(metav1.ListOptions{})

	if err != nil {
		panic(err.Error())
	}
	var filteredServices []corev1.Service
	for _,service:= range services.Items {
		domainName := service.Annotations["auto-dns"]
		if domainName != "" {
			//Only allow Services that exposes something
			if service.Spec.Type == corev1.ServiceTypeNodePort || service.Spec.Type == corev1.ServiceTypeLoadBalancer {
				filteredServices = append(filteredServices,service)
			}

		}
	}
	filterByServiceList(clientset,filteredServices,domainsNodeList)

}

func filterByServiceList(clientset *kubernetes.Clientset,services []corev1.Service,domainsNodeList *domainNodeList) {
	for _,service:= range services {
		domainName := service.Annotations["auto-dns"]
		cfProxy := false
		proxiedAnnotation := service.Annotations["auto-dns-proxy"]
		if(proxiedAnnotation == ""){
			cfProxy = false
		}else{
			cfProxyParsed,error := strconv.ParseBool(proxiedAnnotation)
			if error == nil {
				cfProxy = cfProxyParsed
			}
		}

		if len(service.Spec.Selector) == 0 {
			continue;
		}
		set := labels.Set(service.Spec.Selector)
		pods,err := clientset.CoreV1().Pods(service.Namespace).List(metav1.ListOptions{LabelSelector:set.AsSelector().String(),FieldSelector:"status.phase=Running"})
		if err != nil {
			panic(err.Error())
		}

		for _,pod := range pods.Items {
			if domainName != ""{
				domainNode := findDomainNode(domainsNodeList.items,domainName)
				if domainNode == nil {
					domainNode = &DomainNode{
						domainName:domainName,
						nodes: []string{pod.Spec.NodeName},
						proxy: cfProxy,
					}
					domainsNodeList.items = append(domainsNodeList.items,domainNode)
				}else{
					if !existNodeIP(domainNode.nodes,pod.Spec.NodeName){
						domainNode.nodes = append(domainNode.nodes,pod.Spec.NodeName)
					}
				}

			}
		}
	}
}


func existNodeIP(hosts []string,hostName string) bool {
	for _,host := range hosts {
		if host == hostName {
			return true
		}
	}
	return false
}

func findDomainNode(nodes []*DomainNode,domainName string) *DomainNode {
   for _,domain := range nodes {
   	 if domain.domainName == domainName {
   	 	return domain
	 }
   }
   return nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
