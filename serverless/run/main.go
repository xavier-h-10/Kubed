package main

import (
	"minik8s/serverless/src/function"
	"minik8s/serverless/src/registry"
)

func main() {
	registry.InitRegistry()                                              // only run on master node
	function.InitFunction("helloworld", "default", "../src/app/func.py") // the third parameter need to be replaced

}
