/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloud_provider

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/controller"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/cloud-provider-baiducloud/pkg/sdk/bce"
	"k8s.io/cloud-provider-baiducloud/pkg/sdk/clientset"
)

// ProviderName is the name of this cloud provider.
const ProviderName = "cce"

// CceUserAgent is prefix of http header UserAgent
const CceUserAgent = "cce-k8s:"

// Baiducloud defines the main struct
type Baiducloud struct {
	CloudConfig
	clientSet  clientset.Interface
	kubeClient kubernetes.Interface
}

// CloudConfig is the cloud config
type CloudConfig struct {
	ClusterID       string `json:"ClusterId"`
	ClusterName     string `json:"ClusterName"`
	AccessKeyID     string `json:"AccessKeyID"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Region          string `json:"Region"`
	VpcID           string `json:"VpcId"`
	SubnetID        string `json:"SubnetId"`
	MasterID        string `json:"MasterId"`
	Endpoint        string `json:"Endpoint"`
	NodeIP          string `json:"NodeIP"`
	Debug           bool   `json:"Debug"`
}

type NodeAnnotation struct {
	VpcId           string `json:"vpcId"`
	VpcRouteTableId string `json:"vpcRouteTableId"`
	VpcRouteRuleId  string `json:"vpcRouteRuleId"`
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(configReader io.Reader) (cloudprovider.Interface, error) {
		var cloud Baiducloud
		var cloudConfig CloudConfig
		configContents, err := ioutil.ReadAll(configReader)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(configContents, &cloudConfig)
		if err != nil {
			return nil, err
		}
		glog.V(3).Infof("Init CCE cloud with cloudConfig: %v\n", cloudConfig)
		if cloudConfig.MasterID == "" {
			return nil, fmt.Errorf("Cloud config mast have a Master ID\n")
		}
		if cloudConfig.ClusterID == "" {
			return nil, fmt.Errorf("Cloud config mast have a ClusterID\n")
		}
		if cloudConfig.Endpoint == "" {
			return nil, fmt.Errorf("Cloud config mast have a Endpoint\n")
		}
		cred := bce.NewCredentials(cloudConfig.AccessKeyID, cloudConfig.SecretAccessKey)
		bceConfig := bce.NewConfig(cred)
		bceConfig.Region = cloudConfig.Region
		// timeout need to set
		bceConfig.Timeout = 10 * time.Second
		// fix endpoint
		fixEndpoint := cloudConfig.Endpoint + "/internal-api"
		bceConfig.Endpoint = fixEndpoint
		// http request from cce's kubernetes has an useragent header
		// example: useragent: cce-k8s:c-adfdf
		bceConfig.UserAgent = CceUserAgent + cloudConfig.ClusterID
		cloud.CloudConfig = cloudConfig
		cloud.clientSet, err = clientset.NewFromConfig(bceConfig)
		if err != nil {
			return nil, err
		}
		cloud.clientSet.Blb().SetDebug(true)
		cloud.clientSet.Eip().SetDebug(true)
		cloud.clientSet.Bcc().SetDebug(true)
		cloud.clientSet.Cce().SetDebug(true)
		cloud.clientSet.Vpc().SetDebug(true)
		return &cloud, nil
	})
}

// ProviderName returns the cloud provider ID.
func (bc *Baiducloud) ProviderName() string {
	return ProviderName
}

// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
// to perform housekeeping activities within the cloud provider.
func (bc *Baiducloud) Initialize(clientBuilder controller.ControllerClientBuilder) {
	bc.kubeClient = clientBuilder.ClientOrDie(ProviderName)
}

func (bc *Baiducloud) SetInformers(informerFactory informers.SharedInformerFactory) {
	glog.V(3).Infof("Setting up informers for Baiducloud")
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			glog.V(3).Infof("Node add: ", node.String())
		},
		UpdateFunc: func(prev, obj interface{}) {
			node := obj.(*v1.Node)
			glog.V(3).Infof("Node update: ", node.String())
		},
		DeleteFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			glog.V(3).Infof("Node delete: ", node.String())
		},
	})
}
