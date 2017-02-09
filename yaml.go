package main

import (
	"fmt"
	"io"

	"github.com/ghodss/yaml"
	"k8s.io/kubernetes/pkg/runtime"
)

// YAMLPrinter is an implementation of ResourcePrinter which outputs an object as YAML.
// The input object is assumed to be in the internal version of an API and is converted
// to the given version first.
type YAMLPrinter struct {
	version   string
	converter runtime.ObjectConvertor
}

func (p *YAMLPrinter) AfterPrint(w io.Writer, res string) error {
	return nil
}

// PrintObj prints the data as YAML.
func (p *YAMLPrinter) PrintObj(obj runtime.Object, w io.Writer) error {
	switch obj := obj.(type) {
	case *runtime.Unknown:
		data, err := yaml.JSONToYAML(obj.Raw)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	output, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, string(output))
	return err
}

// TODO: implement HandledResources()
func (p *YAMLPrinter) HandledResources() []string {
	return []string{}
}
