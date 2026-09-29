---
title: "Functions"
sidebar_position: 3
---

# Functions

<!-- Generated from the engine's function table by TestFunctionsDoc; do not edit. -->

012 has 142 functions. They follow Google Sheets' names, arguments and
semantics; `[brackets]` mark optional arguments. Function names are
case-insensitive, and 1-2-3's `@SUM(A1..A5)` spelling still works.

Aliases: `AVG` for `AVERAGE`.

## Everyday

| Function | Description |
|---|---|
| `ABS(value)` | Absolute value |
| `AND(logical1, [logical2, ...])` | TRUE if all arguments are true |
| `AVERAGE(value1, [value2, ...])` | Average of numbers, ignoring text |
| `COUNT(value1, [value2, ...])` | Count of numeric values, skipping errors |
| `COUNTA(value1, [value2, ...])` | Count of non-empty values, errors included |
| `FALSE()` | The logical value FALSE |
| `IF(condition, value_if_true, [value_if_false])` | Choose a value by a condition |
| `IFERROR(value, [value_if_error])` | A fallback when a value is an error; blank without one |
| `INT(value)` | Round down to the nearest integer |
| `MAX(value1, [value2, ...])` | Largest number |
| `MIN(value1, [value2, ...])` | Smallest number |
| `MOD(dividend, divisor)` | Remainder, with the sign of the divisor: MOD(0.3, 0.1) is 0.1 as in Excel, where Sheets gives -5.55E-17 |
| `NA()` | The #N/A error |
| `NOT(logical)` | The opposite of a logical value |
| `OR(logical1, [logical2, ...])` | TRUE if any argument is true |
| `PI()` | The number pi |
| `ROUND(value, [places])` | Round to a number of decimal places, halves away from zero |
| `SQRT(value)` | Square root |
| `SUM(value1, [value2, ...])` | Sum of numbers |
| `TRUE()` | The logical value TRUE |

## Math

| Function | Description |
|---|---|
| `CEILING(value, [factor])` | Round up to a multiple of factor |
| `EXP(exponent)` | e raised to a power |
| `FLOOR(value, [factor])` | Round down to a multiple of factor |
| `LN(value)` | Natural logarithm |
| `LOG(value, [base])` | Logarithm, base 10 by default |
| `LOG10(value)` | Base-10 logarithm |
| `POWER(base, exponent)` | A number raised to a power, odd roots of negatives included |
| `PRODUCT(factor1, [factor2, ...])` | Product of numbers |
| `QUOTIENT(dividend, divisor)` | Integer part of a division |
| `RAND()` | A random number from 0 up to 1, new on every change (recalculates on every change) |
| `RANDBETWEEN(low, high)` | A random integer between two values, new on every change (recalculates on every change) |
| `ROUNDDOWN(value, [places])` | Round toward zero |
| `ROUNDUP(value, [places])` | Round away from zero |
| `SIGN(value)` | 1, 0 or -1 by the sign of a number |
| `SUMIF(range, criterion, [sum_range])` | Sum of the cells that meet a condition |
| `SUMIFS(sum_range, criteria_range1, criterion1, [criteria_range2, criterion2, ...])` | Sum of the cells that meet every condition |
| `SUMPRODUCT(array1, [array2, ...])` | Sum of the products of matching entries, TRUE counting as 1 |
| `TRUNC(value, [places])` | Drop decimals past a number of places |

## Statistics

| Function | Description |
|---|---|
| `AVERAGEIF(criteria_range, criterion, [average_range])` | Average of the cells that meet a condition |
| `AVERAGEIFS(average_range, criteria_range1, criterion1, [criteria_range2, criterion2, ...])` | Average of the cells that meet every condition |
| `COUNTBLANK(range)` | Count of empty cells, including empty text |
| `COUNTIF(range, criterion)` | Count of the cells that meet a condition |
| `COUNTIFS(criteria_range1, criterion1, [criteria_range2, criterion2, ...])` | Count of the cells that meet every condition |
| `LARGE(data, n)` | The nth largest number |
| `MEDIAN(value1, [value2, ...])` | Middle value of numbers |
| `MODE(value1, [value2, ...])` | Most common number (the first, on a tie) |
| `RANK(value, data, [is_ascending])` | Rank of a number among others, largest first by default |
| `SMALL(data, n)` | The nth smallest number |
| `STDEV(value1, [value2, ...])` | Standard deviation of a sample |
| `STDEVP(value1, [value2, ...])` | Standard deviation of a whole population |
| `VAR(value1, [value2, ...])` | Variance of a sample |
| `VARP(value1, [value2, ...])` | Variance of a whole population |

## Logic and information

| Function | Description |
|---|---|
| `IFNA(value, value_if_na)` | A fallback when a value is #N/A |
| `IFS(condition1, value1, [condition2, value2, ...])` | The value of the first true condition |
| `ISBLANK(value)` | TRUE if a cell is empty |
| `ISERR(value)` | TRUE if a value is an error other than #N/A |
| `ISERROR(value)` | TRUE if a value is any error |
| `ISLOGICAL(value)` | TRUE if a value is TRUE or FALSE |
| `ISNA(value)` | TRUE if a value is #N/A |
| `ISNUMBER(value)` | TRUE if a value is a number |
| `ISTEXT(value)` | TRUE if a value is text |
| `SWITCH(expression, case1, value1, [case2, value2, ...], [default])` | The value for the first case equal to an expression |
| `XOR(logical1, [logical2, ...])` | TRUE if an odd number of arguments are true |

## Text

| Function | Description |
|---|---|
| `CONCAT(value1, value2)` | Join two values as text |
| `CONCATENATE(string1, [string2, ...])` | Join text, including every cell of ranges |
| `EXACT(string1, string2)` | TRUE if two texts are identical, case included |
| `FIND(search_for, text_to_search, [starting_at])` | Position of text, case-sensitive |
| `LEFT(string, [number_of_characters])` | The first characters of text |
| `LEN(text)` | Number of characters in text: an emoji is one, where Sheets counts two |
| `LOWER(text)` | Text in lower case |
| `MID(string, starting_at, extract_length)` | Characters from the middle of text |
| `PROPER(text)` | Text with each word capitalized |
| `REPLACE(text, position, length, new_text)` | Replace characters at a position |
| `REPT(text, number_of_repetitions)` | Text repeated a number of times |
| `RIGHT(string, [number_of_characters])` | The last characters of text |
| `SEARCH(search_for, text_to_search, [starting_at])` | Position of text, ignoring case, with * and ? wildcards |
| `SUBSTITUTE(text, search_for, replace_with, [occurrence_number])` | Replace occurrences of text |
| `TEXT(number, format)` | A number as text in a format, e.g. "$#,##0.00" or "yyyy-mm-dd" |
| `TEXTJOIN(delimiter, ignore_empty, text1, [text2, ...])` | Join text with a delimiter |
| `TRIM(text)` | Text without leading, trailing and repeated spaces |
| `UPPER(text)` | Text in upper case: ß is SS |
| `VALUE(text)` | Text as a number; dates and times too |

## Split and regular expressions

| Function | Description |
|---|---|
| `REGEXEXTRACT(text, regular_expression)` | The first match of a regular expression, or its capture groups across |
| `REGEXMATCH(text, regular_expression)` | TRUE if text matches a regular expression |
| `REGEXREPLACE(text, regular_expression, replacement)` | Text with every match replaced; $1 in the replacement is a capture group |
| `SPLIT(text, delimiter, [split_by_each], [remove_empty_text])` | Text split at a delimiter into cells across (each character of it by default) |

## Lookup

| Function | Description |
|---|---|
| `CHOOSE(index, choice1, [choice2, ...])` | The choice at a position |
| `COLUMNS(range)` | Number of columns in a range |
| `HLOOKUP(search_key, range, index, [is_sorted])` | Find a key in the first row and return a value from its column |
| `INDEX(reference, [row], [column])` | The value at a row and column of a range; row or column 0 for a whole column or row |
| `MATCH(search_key, range, [search_type])` | Position of a key in a row or column |
| `ROWS(range)` | Number of rows in a range |
| `VLOOKUP(search_key, range, index, [is_sorted])` | Find a key in the first column and return a value from its row |
| `XLOOKUP(search_key, lookup_range, result_range, [missing_value], [match_mode], [search_mode])` | Find a key and return the matching entry of another range |

## Arrays

| Function | Description |
|---|---|
| `ARRAYFORMULA(array_formula)` | Compute a formula over arrays: ranges read whole, and functions of one value applied to each entry |
| `CHOOSECOLS(array, col_num1, [col_num2, ...])` | Columns of an array by position, negative from the end |
| `CHOOSEROWS(array, row_num1, [row_num2, ...])` | Rows of an array by position, negative from the end |
| `FILTER(range, condition1, [condition2, ...])` | The rows (or columns) of a range where every condition is true |
| `FLATTEN(range1, [range2, ...])` | Every entry of ranges in one column, row by row |
| `SEQUENCE(rows, [columns], [start], [step])` | An array of numbers counting up from start by step |
| `SORT(range, [sort_column], [is_ascending], [sort_column2, is_ascending2, ...])` | The rows of a range sorted by columns |
| `SORTN(range, [n], [display_ties_mode], [sort_column1, is_ascending1, ...])` | The first n rows of a range after sorting |
| `TRANSPOSE(array_or_range)` | Rows as columns and columns as rows |
| `UNIQUE(range, [by_column], [exactly_once])` | The distinct rows (or columns) of a range, in order |

## LET and LAMBDA

| Function | Description |
|---|---|
| `BYCOL(array_or_range, LAMBDA)` | Each column of an array passed to a LAMBDA, one value per column |
| `BYROW(array_or_range, LAMBDA)` | Each row of an array passed to a LAMBDA, one value per row |
| `LAMBDA([name, ...], formula_expression)` | A function of names, called with values: LAMBDA(x, x*2)(3) |
| `LET(name1, value_expression1, [name2, value_expression2, ...], formula_expression)` | Name values for use in a formula; an error counts only where its name is used |
| `MAKEARRAY(rows, columns, LAMBDA)` | An array of a size, each entry a LAMBDA of its row and column |
| `MAP(array1, [array2, ...], LAMBDA)` | Each entry of arrays passed to a LAMBDA |
| `REDUCE(initial_value, array_or_range, LAMBDA)` | An array folded into one value by a LAMBDA of the total so far and each entry |
| `SCAN(initial_value, array_or_range, LAMBDA)` | The running totals of REDUCE, one for each entry |

## Date and time

| Function | Description |
|---|---|
| `DATE(year, month, day)` | A date from its parts; months and days past the end roll over |
| `DATEDIF(start_date, end_date, unit)` | Time between dates in "Y", "M", "D", "MD", "YM" or "YD" |
| `DATEVALUE(date_string)` | The date a text such as "2026-09-26" stands for |
| `DAY(date)` | Day of the month of a date |
| `DAYS(end_date, start_date)` | Number of days between two dates |
| `EDATE(start_date, months)` | The same day a number of months away |
| `EOMONTH(start_date, months)` | The last day of the month a number of months away |
| `HOUR(time)` | Hour of a time, 0 to 23 |
| `MINUTE(time)` | Minute of a time, 0 to 59 |
| `MONTH(date)` | Month of a date, 1 to 12 |
| `NETWORKDAYS(start_date, end_date, [holidays])` | Number of weekdays between two dates, counting both |
| `NOW()` | The current date and time, updated on every change (recalculates on every change) |
| `SECOND(time)` | Second of a time, 0 to 59 |
| `TIME(hour, minute, second)` | A time of day from its parts |
| `TIMEVALUE(time_string)` | The time of day a text such as "2:30 PM" stands for |
| `TODAY()` | Today's date, updated on every change (recalculates on every change) |
| `WEEKDAY(date, [type])` | Day of the week: 1 is Sunday, or Monday with type 2 |
| `YEAR(date)` | Year of a date |

## Finance

| Function | Description |
|---|---|
| `FV(rate, number_of_periods, payment_amount, [present_value], [end_or_beginning])` | Future value of a series of payments |
| `IRR(cashflow_amounts, [rate_guess])` | Internal rate of return of periodic cash flows |
| `NPER(rate, payment_amount, present_value, [future_value], [end_or_beginning])` | Number of periods to pay off a loan or reach a value |
| `NPV(discount, cashflow1, [cashflow2, ...])` | Net present value of periodic cash flows |
| `PMT(rate, number_of_periods, present_value, [future_value], [end_or_beginning])` | Payment per period of a loan or investment |
| `PV(rate, number_of_periods, payment_amount, [future_value], [end_or_beginning])` | Present value of a series of payments |
| `RATE(number_of_periods, payment_per_period, present_value, [future_value], [end_or_beginning], [rate_guess])` | Interest rate per period of an annuity |

## Links

| Function | Description |
|---|---|
| `HYPERLINK(url, [link_label])` | A link that opens url, shown as its label |

## JEV (hosted model)

| Function | Description |
|---|---|
| `JEV.CLASSIFY(value, question, labels, [descriptions])` | The label that fits best, chosen by the JEV model (recalculates on every change) |
| `JEV.PROB(value, question, [yes_means], [no_means])` | The probability the answer is yes, from the JEV model (recalculates on every change) |
| `JEV.SCORE(value, question, levels)` | A score on an ordered rubric, from the JEV model (recalculates on every change) |
| `JEV.TEST(value, question, [yes_means], [no_means])` | TRUE or FALSE, answered by the JEV model (recalculates on every change) |

