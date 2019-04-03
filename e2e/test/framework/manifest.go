package framework

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
)

type ManifestTemplate struct {
	Image string
}

func (f *Framework) ApplyManifest() error {
	manifests := []string{
		"crd",
		"service-account",
		"cluster-role",
		"crd-object",
		"storage-class",
	}

	for _, manifest := range manifests {
		data, err := f.readManifest(manifest)
		if err != nil {
			return err
		}

		err = ApplyManifest("apply", data)
		if err != nil {
			return err
		}
	}

	data, err := f.readCSIManifest()
	if err != nil {
		return err
	}

	return ApplyManifest("apply", data)
}

func (f *Framework) DeleteManifest() error {
	data, err := f.readCSIManifest()
	if err != nil {
		return err
	}

	err = ApplyManifest("delete", data)
	if err != nil {
		return err
	}

	manifests := []string{
		"storage-class",
		"crd-object",
		"cluster-role",
		"service-account",
		"crd",
	}

	for _, manifest := range manifests {
		data, err := f.readManifest(manifest)
		if err != nil {
			return err
		}

		err = ApplyManifest("delete", data)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *Framework) readCSIManifest() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(filepath.Join(dir, "manifests/csi-linode.yaml"))
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("csidriver").Parse(string(data))
	if err != nil {
		return "", err
	}
	var tmplBuf bytes.Buffer
	err = tmpl.Execute(&tmplBuf, ManifestTemplate{
		Image: Image,
	})
	if err != nil {
		return "", err
	}
	return tmplBuf.String(), nil
}

func (f *Framework) readManifest(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(filepath.Join(dir, "manifests/"+name+".yaml"))
	if err != nil {
		return "", err
	}

	return string(data), nil
}
