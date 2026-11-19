#!/bin/bash
# Limpiar caracteres especiales en archivos Go

for file in $(find . -name "*.go"); do
    echo "Limpiando $file"
    # Convertir a UTF-8 puro y eliminar caracteres no imprimibles
    iconv -f utf-8 -t ascii//TRANSLIT//IGNORE "$file" > "$file.clean"
    # Eliminar caracteres de control (excepto tabs y newlines)
    perl -pe 's/[^[:print:]\t\n]//g' "$file.clean" > "$file"
    rm "$file.clean"
done

echo "✅ Archivos limpiados"