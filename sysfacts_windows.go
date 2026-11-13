//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// readSysFacts asks Windows for its version and the PC's make, model, processor and memory.
func readSysFacts() map[string]string {
	out, err := powershell(`$ErrorActionPreference='SilentlyContinue'
$o=Get-CimInstance Win32_OperatingSystem; $c=Get-CimInstance Win32_ComputerSystem; $p=Get-CimInstance Win32_Processor | Select-Object -First 1
$v=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
[pscustomobject]@{os=[string]$o.Caption; osVersion=[string]$v.DisplayVersion; build=[string]$o.BuildNumber+'.'+[string]$v.UBR;
 maker=[string]$c.Manufacturer; model=[string]$c.Model; cpu=[string]$p.Name; memory=[string]([math]::Round($c.TotalPhysicalMemory/1GB,1))+' GB';
 lang=[string](Get-Culture).Name} | ConvertTo-Json -Compress`)
	f := map[string]string{}
	if logErr("read the Windows version", err) != nil {
		return f
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &f); err != nil {
		logErr("read the Windows version", fmt.Errorf("%v: %s", err, firstLine(string(out))))
	}
	for k, v := range f {
		f[k] = strings.TrimSpace(v)
	}
	return f
}
