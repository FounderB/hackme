' Run pool worker watchdog with no console window (avoids blank "hackme-watchdog" CMD).
' Logs: logs\watchdog_worker.log next to hackme.exe
Option Explicit
Dim sh, fso, dir, bat, rc
Set sh = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
dir = fso.GetParentFolderName(WScript.ScriptFullName)
bat = dir & "\watchdog_pool_worker.bat"
If Not fso.FileExists(bat) Then
  WScript.Quit 1
End If
sh.CurrentDirectory = dir
' WindowStyle 0 = hidden, False = asynchronous
rc = sh.Run("cmd.exe /c call """ & bat & """", 0, False)
WScript.Quit 0
