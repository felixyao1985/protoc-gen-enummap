package main

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

var formatters = map[string]enumFormatter{
	"csv":   &csvEnumFormatter{},
	"jsonl": &jsonlEnumFormatter{},
	"sql":   &sqlEnumFormatter{},
}

func getNested(d interface{}) []*descriptor.DescriptorProto {
	if fdp, ok := d.(*descriptor.FileDescriptorProto); ok {
		return fdp.GetMessageType()
	} else if dp, ok := d.(*descriptor.DescriptorProto); ok {
		return dp.GetNestedType()
	} else {
		return nil
	}
}

func getDescName(d interface{}) string {
	if fdp, ok := d.(*descriptor.FileDescriptorProto); ok {
		return strings.Replace(fdp.GetPackage(), ".", "_", -1)
	} else if dp, ok := d.(*descriptor.DescriptorProto); ok {
		return dp.GetName() + "_"
	} else {
		return ""
	}
}

func merge(l, r map[string][]ContentEntry) map[string][]ContentEntry {
	merged := make(map[string][]ContentEntry)

	for k, v := range l {
		merged[k] = v
	}
	for k, v := range r {
		if cur, ok := merged[k]; ok {
			merged[k] = append(cur, v...)
		} else {
			merged[k] = v
		}
	}

	return merged
}

func appendNestedEnum(formatter enumFormatter, baseDescName string, desc []*descriptor.DescriptorProto) map[string][]ContentEntry {
	entries := make(map[string][]ContentEntry)

	for _, d := range desc {
		for _, e := range d.GetEnumType() {
			c := ContentEntry{
				EnumValues:  e.GetValue(),
				MessageName: getDescName(d) + e.GetName(),
			}

			v, ok := entries[baseDescName]
			if ok {
				entries[baseDescName] = append(v, c)
			} else {
				entries[baseDescName] = []ContentEntry{c}
			}
		}
		entries = merge(entries, appendNestedEnum(formatter, baseDescName, getNested(d)))
	}

	return entries
}

func main() {
	buf, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	var req plugin.CodeGeneratorRequest
	if err := proto.Unmarshal(buf, &req); err != nil {
		log.Fatal(err)
	}

	formatter, fmtOk := formatters[req.GetParameter()]
	if !fmtOk {
		log.Fatal("Specify supported format by --enummap_opt=")
	}

	entries := make(map[string][]ContentEntry)
	for _, f := range req.GetProtoFile() {
		descName := getDescName(f)
		for _, e := range f.GetEnumType() {
			c := ContentEntry{
				EnumValues:  e.GetValue(),
				MessageName: e.GetName(),
			}

			if v, ok := entries[descName]; ok {
				entries[descName] = append(v, c)
			} else {
				entries[descName] = []ContentEntry{c}
			}
		}
		entries = merge(entries, appendNestedEnum(formatter, descName, getNested(f)))
	}

	resp := plugin.CodeGeneratorResponse{}
	for descName, contentEntries := range entries {
		resp.File = append(resp.File, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(descName + formatter.extension()),
			Content: proto.String(formatter.printContent(descName, contentEntries)),
		})
	}

	buf, err = proto.Marshal(&resp)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(buf); err != nil {
		log.Fatal(err)
	}
}
