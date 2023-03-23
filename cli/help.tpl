{{- /* Heavily inspired by the Go toolchain formatting. */ -}}
{{ with .Short }}
{{- wrapTTY . }}
{{"\n"}}
{{- end}}
Usage:
  {{.FullUsage}}

{{ with .Aliases }}
{{ "\n" }}
{{ "Aliases:"}} {{ joinStrings .}}
{{ "\n" }}
{{- end }}

{{- with .Long}}
{{- formatLong . }}
{{ "\n" }}
{{- end }}
{{ with visibleChildren . }}
{{- range $index, $child := . }}
{{- if eq $index 0 }}
{{ "Commands:"}}
{{- end }}
    {{- "\n" }}
    {{- formatSubcommand . | trimNewline }}
{{- end }}
{{- "\n" }}
{{- end }}
{{- range $index, $group := optionGroups . }}
{{ with $group.Name }} {{- print $group.Name " Flags:" }} {{ else -}} {{ "Flags:"}}{{- end -}}
{{- with $group.Description }}
{{ formatGroupDescription . }}
{{- else }}
{{- end }}
    {{- range $index, $option := $group.Options }}
	{{- if not (eq $option.FlagShorthand "") }}{{- print "\n  -" $option.FlagShorthand ", " -}}
	{{- else }}{{- print "\n      " -}}
	{{- end }}
    {{- with flagName $option }}--{{ . }}{{ end }}
	{{- with typeHelper $option }} {{ . }}{{ end }}
        {{- with $option.Description }}
            {{- $desc := $option.Description }}
{{- indent $desc 10 $option $group.Options }}
{{- if isDeprecated $option }} DEPRECATED {{ end }}
        {{- end -}}
    {{- end }}
{{- end }}
