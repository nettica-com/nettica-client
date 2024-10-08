package main

import (
	"bytes"
	"strings"
	"text/template"

	model "github.com/nettica-com/nettica-admin/model"
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

	wireguardTemplate = `{{ if .Vpn.Enable }}
# {{.Vpn.Name }} / Updated: {{ .Vpn.Updated }} / Created: {{ .Vpn.Created }}
[Interface]
  {{- range .Vpn.Current.Address }}
Address = {{ . }}
  {{- end }}
PrivateKey = {{ .Vpn.Current.PrivateKey }}
{{ $server := .Vpn.Current.Endpoint -}}{{ $service := .Vpn.Current.Type -}}
{{ if ne .Vpn.Current.ListenPort 0 -}}ListenPort = {{ .Vpn.Current.ListenPort }}{{- end}}
{{ if .Vpn.Current.Dns }}DNS = {{ StringsJoin .Vpn.Current.Dns ", " }}{{ end }}
{{ if .Vpn.Current.Table }}Table = {{ .Vpn.Current.Table }}{{- end}}
{{ if ne .Vpn.Current.Mtu 0 -}}MTU = {{.Vpn.Current.Mtu}}{{- end}}
{{ if .Vpn.Current.PreUp -}}PreUp = {{ .Vpn.Current.PreUp }}{{- end}}
{{ if .Vpn.Current.PostUp -}}PostUp = {{ .Vpn.Current.PostUp }}{{- end}}
{{ if .Vpn.Current.PreDown -}}PreDown = {{ .Vpn.Current.PreDown }}{{- end}}
{{ if .Vpn.Current.PostDown -}}PostDown = {{ .Vpn.Current.PostDown }}{{- end}}
{{ range .VPNs -}}
{{ if or .Enable $service -}}
{{ if $server }}
# {{.Name}} / Updated: {{.Updated}} / Created: {{.Created}}
[Peer]
PublicKey = {{ .Current.PublicKey }}
PresharedKey = {{ .Current.PresharedKey }}
AllowedIPs = {{ StringsJoin .Current.AllowedIPs ", " }}
{{ if .Current.Endpoint -}}Endpoint = {{ .Current.Endpoint }} {{- end }}
{{ if .Current.PersistentKeepalive -}}PersistentKeepalive = {{ .Current.PersistentKeepalive }}{{ end }}
{{ else -}}
{{ if .Current.Endpoint -}}
# {{.Name}} / Updated: {{.Updated}} / Created: {{.Created}}
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
func DumpWireguardConfig(vpn *model.VPN, VPNs *[]model.VPN) ([]byte, error) {
	t, err := template.New("wireguard").Funcs(template.FuncMap{"StringsJoin": strings.Join}).Parse(wireguardTemplate)
	if err != nil {
		return nil, err
	}

	return dump(t, struct {
		Vpn  *model.VPN
		VPNs *[]model.VPN
	}{
		Vpn:  vpn,
		VPNs: VPNs,
	})
}

// DumpClientWg dump client wg config with go template
func DumpClientWg(vpn *model.VPN, server *model.Server) ([]byte, error) {
	t, err := template.New("client").Funcs(template.FuncMap{"StringsJoin": strings.Join}).Parse(clientTpl)
	if err != nil {
		return nil, err
	}

	return dump(t, struct {
		Vpn    *model.VPN
		Server *model.Server
	}{
		Vpn:    vpn,
		Server: server,
	})
}

func dump(tpl *template.Template, data interface{}) ([]byte, error) {
	var tplBuff bytes.Buffer

	err := tpl.Execute(&tplBuff, data)
	if err != nil {
		return nil, err
	}

	return tplBuff.Bytes(), nil
}
