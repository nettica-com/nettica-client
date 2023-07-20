package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	model "github.com/nettica-com/nettica-admin/model"
	util "github.com/nettica-com/nettica-admin/util"
)

var (
	clientTpl = `[Interface]
Address = {{ StringsJoin .Host.Current.Address ", " }}
PrivateKey = {{ .Host.Current.PrivateKey }}
{{ if ne (len .Server.Dns) 0 -}}
DNS = {{ StringsJoin .Server.Dns ", " }}
{{- end }}
{{ if ne .Server.Mtu 0 -}}
MTU = {{.Server.Mtu}}
{{- end}}
[Peer]
PublicKey = {{ .Host.Current.PublicKey }}
PresharedKey = {{ .Host.Current.PresharedKey }}
AllowedIPs = {{ StringsJoin .Host.Current.AllowedIPs ", " }}
Endpoint = {{ .Server.Endpoint }}
PersistentKeepalive = {{.Host.Current.PersistentKeepalive}}
`

	wgTpl = `# Updated: {{ .Server.Updated }} / Created: {{ .Server.Created }}
[Interface]
{{- range .Server.Address }}
Address = {{ . }}
{{- end }}
ListenPort = {{ .Server.ListenPort }}
PrivateKey = {{ .Server.PrivateKey }}
{{ if ne .Server.Mtu 0 -}}
MTU = {{.Server.Mtu}}
{{- end}}
PreUp = {{ .Server.PreUp }}
PostUp = {{ .Server.PostUp }}
PreDown = {{ .Server.PreDown }}
PostDown = {{ .Server.PostDown }}
{{- range .Hosts }}
{{ if .Enable -}}
# {{.Name}} / {{.Email}} / Updated: {{.Updated}} / Created: {{.Created}}
[Peer]
PublicKey = {{ .Current.PublicKey }}
PresharedKey = {{ .Current.PresharedKey }}
AllowedIPs = {{ StringsJoin .Current.Address ", " }}
{{- end }}
{{ end }}`

	wireguardTemplate = `{{ if .Host.Enable }}
# {{.Host.Name }} / {{ .Host.Email }} / Updated: {{ .Host.Updated }} / Created: {{ .Host.Created }}
[Interface]
  {{- range .Host.Current.Address }}
Address = {{ . }}
  {{- end }}
PrivateKey = {{ .Host.Current.PrivateKey }}
{{ $server := .Host.Current.Endpoint -}}
{{ if ne .Host.Current.ListenPort 0 -}}ListenPort = {{ .Host.Current.ListenPort }}{{- end}}
{{ if .Host.Current.Dns }}DNS = {{ StringsJoin .Host.Current.Dns ", " }}{{ end }}
{{ if .Host.Current.Table }}Table = {{ .Host.Current.Table }}{{- end}}
{{ if ne .Host.Current.Mtu 0 -}}MTU = {{.Host.Current.Mtu}}{{- end}}
{{ if .Host.Current.PreUp -}}PreUp = {{ .Host.Current.PreUp }}{{- end}}
{{ if .Host.Current.PostUp -}}PostUp = {{ .Host.Current.PostUp }}{{- end}}
{{ if .Host.Current.PreDown -}}PreDown = {{ .Host.Current.PreDown }}{{- end}}
{{ if .Host.Current.PostDown -}}PostDown = {{ .Host.Current.PostDown }}{{- end}}
{{ range .Hosts -}}
{{ if .Enable -}}
{{ if $server }}
# {{.Name}} / {{.Email}} / Updated: {{.Updated}} / Created: {{.Created}}
[Peer]
PublicKey = {{ .Current.PublicKey }}
PresharedKey = {{ .Current.PresharedKey }}
AllowedIPs = {{ StringsJoin .Current.AllowedIPs ", " }}
{{ if .Current.Endpoint -}}Endpoint = {{ .Current.Endpoint }} {{- end }}
{{ if .Current.PersistentKeepalive -}}PersistentKeepalive = {{ .Current.PersistentKeepalive }}{{ end }}
{{ else -}}
{{ if .Current.Endpoint -}}
# {{.Name}} / {{.Email}} / Updated: {{.Updated}} / Created: {{.Created}}
[Peer]
PublicKey = {{ .Current.PublicKey }}
PresharedKey = {{ .Current.PresharedKey }}
AllowedIPs = {{ StringsJoin .Current.AllowedIPs ", " }}
{{ if .Current.Endpoint -}}Endpoint = {{ .Current.Endpoint }} {{- end }}
{{ if .Current.PersistentKeepalive }}PersistentKeepalive = {{ .Current.PersistentKeepalive }}{{ end }}
{{ end }}
{{ end -}}
{{ end -}}
{{ end -}}
{{ end }}`
)

// DumpWireguardConfig using go template
func DumpWireguardConfig(host *model.Host, hosts *[]model.Host) ([]byte, error) {
	t, err := template.New("wireguard").Funcs(template.FuncMap{"StringsJoin": strings.Join}).Parse(wireguardTemplate)
	if err != nil {
		return nil, err
	}

	return dump(t, struct {
		Host  *model.Host
		Hosts *[]model.Host
	}{
		Host:  host,
		Hosts: hosts,
	})
}

// DumpClientWg dump client wg config with go template
func DumpClientWg(host *model.Host, server *model.Server) ([]byte, error) {
	t, err := template.New("client").Funcs(template.FuncMap{"StringsJoin": strings.Join}).Parse(clientTpl)
	if err != nil {
		return nil, err
	}

	return dump(t, struct {
		Host   *model.Host
		Server *model.Server
	}{
		Host:   host,
		Server: server,
	})
}

// DumpServerWg dump server wg config with go template, write it to file and return bytes
func DumpServerWg(hosts []*model.Host, server *model.Server) ([]byte, error) {
	t, err := template.New("server").Funcs(template.FuncMap{"StringsJoin": strings.Join}).Parse(wgTpl)
	if err != nil {
		return nil, err
	}

	configDataWg, err := dump(t, struct {
		Hosts  []*model.Host
		Server *model.Server
	}{
		Hosts:  hosts,
		Server: server,
	})
	if err != nil {
		return nil, err
	}

	err = util.WriteFile(filepath.Join(os.Getenv("WG_CONF_DIR"), os.Getenv("WG_INTERFACE_NAME")), configDataWg)
	if err != nil {
		return nil, err
	}

	return configDataWg, nil
}

func dump(tpl *template.Template, data interface{}) ([]byte, error) {
	var tplBuff bytes.Buffer

	err := tpl.Execute(&tplBuff, data)
	if err != nil {
		return nil, err
	}

	return tplBuff.Bytes(), nil
}
