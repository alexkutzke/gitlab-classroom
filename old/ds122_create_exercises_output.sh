#!/bin/sh

students_file=${1}
output_file="/tmp/output_`date '+%N'`"
final_output_file="/tmp/final_output_`date '+%N'`"

echo "|Nome|" > ${output_file}
echo "|----|" >> ${output_file}

while read grr name
do
  echo "|${name}|" >> ${output_file}
done < ${students_file}

files=""
for repo in ${@:2}
do
  files="${files} ${repo}/output.md"
done

paste ${output_file} ${files}

