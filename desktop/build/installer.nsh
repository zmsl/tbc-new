; Custom NSIS hooks. electron-builder picks this up automatically as buildResources/installer.nsh.
;
; The app ships two executables, not one: the Electron shell, and wowsimtbc.exe -- the Go sim
; engine, which the shell runs as a child process out of the install directory. electron-builder's
; "is the app running" check only knows about the shell, so anything holding the engine's file open
; is invisible to it. The installer then fails to copy over that file and reports the only thing it
; can: "WoWSims TBC cannot be closed. Please close it manually and click Retry" -- naming an app the
; user has already closed and cannot find in the task list.
;
; The engine normally exits with the shell (it watches stdin for EOF, see sim/web/main.go), so this
; is a backstop for the cases where it cannot: a shell killed in a way that left the pipe open, a
; wedged engine, or an install over a version that predates that watcher.
;
; Note this matches by process name, so it would also stop a standalone wowsimtbc.exe server the
; user happened to be running from a separate download. That is the right trade for an installer
; that would otherwise fail: the server is stateless and restarting it costs nothing.

!macro closeSimEngine
  ; Ask first. Harmless if nothing is running -- the return code is ignored either way.
  ${nsProcess::CloseProcess} "wowsimtbc.exe" $R9
  Sleep 300
  ; Then insist. A wedged engine still holds the file, and by this point the install cannot
  ; proceed without it.
  ${nsProcess::KillProcess} "wowsimtbc.exe" $R9
  ${nsProcess::Unload}
!macroend

; Runs in .onInit, before the app-running check and well before any file is written.
!macro customInit
  !insertmacro closeSimEngine
!macroend

; Same problem in reverse: a running engine keeps its own file from being deleted, leaving a
; half-removed install directory for the next install to trip over.
!macro customUnInit
  !insertmacro closeSimEngine
!macroend
