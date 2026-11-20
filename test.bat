@echo off
node "%~dp0scripts\test-all.mjs" %*
exit /b %ERRORLEVEL%
