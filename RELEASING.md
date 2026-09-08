# Cómo cortar una release

1. Actualizar la versión en dos lugares (deben coincidir):
   - `internal/version/version.go` → `const Current = "X.Y.Z"`
   - `wails.json` → `"info"."productVersion"`
2. Commitear ese cambio.
3. Taggear y pushear:
   ```
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```
4. El workflow `.github/workflows/release.yml` se dispara solo con el push del tag: compila el instalador NSIS y el binario suelto, y publica ambos como GitHub Release con los nombres fijos que espera el auto-updater:
   - `koalmine-windows-amd64.exe` — binario suelto, usado por el auto-update para reemplazar la instalación existente.
   - `koalmine-windows-amd64-installer.exe` — instalador NSIS, para instalaciones nuevas.

El tag **debe** empezar con `v` (`v0.1.0`, no `0.1.0`) — así lo espera tanto el filtro del workflow (`tags: ["v*"]`) como la comparación de versiones semver del auto-updater (`internal/updater`).

No hay firma de código (Authenticode): Windows va a mostrar el aviso de "editor no verificado" en el instalador, y SmartScreen puede marcar el binario descargado la primera vez. No hay certificado disponible por ahora.
