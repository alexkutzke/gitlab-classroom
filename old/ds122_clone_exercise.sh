#!/bin/sh

group=${1}
repo=${2}
due_date=${3}
students_file=${4}
output_file=${5}
  
[[ ! -d ${repo} ]] && mkdir ${repo}

echo "${repo}<br>${due_date}|" > ${output_file}
echo ":---:|" >> ${output_file}

while read grr name
do
  echo -n "${grr} - ${name} ... "
  student_repo="${repo}/${name}-${grr}-${repo}"
  if [[ ! -d ${student_repo} ]]
  then
    if ! git clone ssh://git@gitlab.com/${group}-${grr}/${repo} ${student_repo} &> /dev/null
    then
      echo " Fork não encontrado |" >> ${output_file}
      echo " Fork não encontrado"
    fi
  fi
  if [[ -d ${student_repo} ]]
  then
    cd "${student_repo}"
    git pull &> /dev/null
    sha=`git log --until=${due_date} | grep commit | head -1 | cut -d' ' -f2`
    git checkout ${sha} &> /dev/null
    if ! git log --pretty=format:%an | grep -v Kutzke &> /dev/null
    then
      cd - &> /dev/null
      echo " Fork, mas nenhum commit até data de entrega|" >> ${output_file}
      echo " Fork, mas nenhum commit até data de entrega"
    else
      cd - &> /dev/null
      echo " ok |" >> ${output_file}
      echo " ok" 
    fi
  fi
done < ${students_file}
