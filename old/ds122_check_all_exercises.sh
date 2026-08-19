#!/bin/sh

if [ $# -lt 4 ]; then
    echo "Not all arguments provided"
    echo "check_all.sh <group> <students_file> <exercises_file> <output_file>"
    echo
    echo " - group: name of the groups minus the grr part"
    echo " - students_file: file containing students data:"
    echo "                  grr name"
    echo " - exercises_file: file containing exercises data:"
    echo "                  name due_date"
    echo " - output_file: file that will receive the output data in markdown format"
    echo "Example: check_all.sh ds122-2021-2-t students.txt exercises.txt ../material/report_t.md"
    exit 1
fi

group=${1}
students_file=${2}
exercises_file=${3}
output_file=${4}

exercises=""
while read exercise due_date
do
  [[ ! -d $exercise ]] && mkdir ${exercise}
  [[ ! -f ${exercise}/output.md ]] && ./ds122_clone_exercise.sh ${group} ${exercise} ${due_date} ${students_file} ${exercise}/output.md
  exercises="${exercises} ${exercise}"
done < ${exercises_file}

echo "# Entrega dos exercícios" > ${output_file} 
echo >> ${output_file} 
echo "- **Grupo**: ${group}" >> ${output_file}  
echo "- **Última atualização**: `date`" >> ${output_file} 
echo >> ${output_file} 

./ds122_create_exercises_output.sh ${students_file} $exercises >> ${output_file} 
