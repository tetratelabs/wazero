#!/bin/bash

t="tetratelabs\/wazero"
h="heeus\/hwazero"

rep() {
  farray=()
  for filename in $1/*.go
  do
     farray+=("$filename")
  done
  for filename in $1/*.mod
  do
     farray+=("$filename")
  done

  for ix in ${!farray[*]}
  do
  # replace <year>,  to current year value in files
    if [ -f ${farray[$ix]} ]
    then
      sed -i "s/$t/$h/" ${farray[$ix]}
    fi
  done

  for dirname in $1/* 
  do 
     dir=$dirname
     if [ -d "$dirname" ]
     then
       rep $dir 
     fi
  done
}  

rep "./."

read -p "Press [Enter] to finish."
