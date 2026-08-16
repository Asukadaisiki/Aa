@echo off
setlocal
set "AA_ROOT=%~dp0"
set "AA_ROOT=%AA_ROOT:~0,-1%"
go run "%AA_ROOT%\cmd\a2a_agent-tui" -config "%AA_ROOT%\configs\config.example.yaml" -workdir "%AA_ROOT%" %*
endlocal
