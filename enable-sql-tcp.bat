@echo off
:: Batch script to enable TCP/IP on SQL Server and restart MSSQLSERVER
:: Run this file as Administrator (Right click -> Run as administrator)
echo ========================================================
echo  Veracity Maps - Enabling SQL Server TCP/IP on Port 1433
echo ========================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$regPath = 'HKLM:\SOFTWARE\Microsoft\Microsoft SQL Server\MSSQL15.MSSQLSERVER\MSSQLServer\SuperSocketNetLib\Tcp'; " ^
  "Set-ItemProperty -Path $regPath -Name 'Enabled' -Value 1; " ^
  "Set-ItemProperty -Path ($regPath + '\IPAll') -Name 'TcpPort' -Value '1433'; " ^
  "Set-ItemProperty -Path ($regPath + '\IPAll') -Name 'TcpDynamicPorts' -Value ''; " ^
  "Write-Host '[OK] TCP/IP enabled on port 1433 in Registry.' -ForegroundColor Green; " ^
  "Write-Host 'Restarting SQL Server service...' -ForegroundColor Yellow; " ^
  "Restart-Service MSSQLSERVER -Force; " ^
  "Write-Host '[SUCCESS] MSSQLSERVER restarted and listening on port 1433!' -ForegroundColor Green;"

if %errorlevel% neq 0 (
    echo.
    echo [ERROR] Failed to modify settings. Did you right click and select 'Run as administrator'?
) else (
    echo.
    echo All done! You can now start the backend with:
    echo   cd backend
    echo   go run main.go
)

echo.
pause

