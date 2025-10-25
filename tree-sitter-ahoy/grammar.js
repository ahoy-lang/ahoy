module.exports = grammar({
  name: 'ahoy',

  extras: $ => [
    /\s/,
    $.comment,
  ],

  conflicts: $ => [
    [$.call_expression, $.expression],
    [$.array_access, $.array_literal],
    [$.dict_access, $.dict_literal],
    [$._statement, $.if_statement],
    [$._statement, $.case_statement],
  ],

  rules: {
    source_file: $ => seq(
      repeat(choice(';', '\n')),
      optional(seq(
        $._statement,
        repeat(seq(
          repeat1(choice(';', '\n')),
          $._statement,
        )),
        repeat(choice(';', '\n')),
      )),
    ),

    _statement: $ => choice(
      $.import_statement,
      $.function_declaration,
      $.variable_declaration,
      $.if_statement,
      $.switch_statement,
      $.loop_statement,
      $.when_statement,
      $.return_statement,
      $.call_expression,
    ),

    comment: $ => token(seq('?', /.*/)),

    // Import statement
    import_statement: $ => seq(
      'import',
      field('path', $.string),
    ),

    // Function declaration
    function_declaration: $ => seq(
      'func',
      field('name', $.identifier),
      '|',
      optional($.parameter_list),
      '|',
      optional(field('return_type', $.type)),
      'then',
      field('body', $.block),
    ),

    parameter_list: $ => seq(
      $.parameter,
      repeat(seq(',', $.parameter)),
    ),

    parameter: $ => seq(
      field('name', $.identifier),
      field('type', $.type),
    ),

    type: $ => choice(
      'int',
      'float',
      'string',
      'bool',
      'dict',
    ),

    // Variable declaration
    variable_declaration: $ => seq(
      field('name', $.identifier),
      ':',
      field('value', $.expression),
    ),

    // If statement (supports inline and multi-line)
    if_statement: $ => prec.right(seq(
      'if',
      field('condition', $.expression),
      'then',
      field('consequence', choice($.block, $.call_expression)),
      optional(choice(
        seq(
          choice('elseif', 'anif'),
          field('alternative_condition', $.expression),
          'then',
          field('alternative', choice($.block, $.call_expression)),
        ),
        seq(
          'else',
          optional('then'),
          field('alternative', choice($.block, $.call_expression)),
        ),
      )),
    )),

    // Switch statement
    switch_statement: $ => prec.right(seq(
      'switch',
      field('value', $.expression),
      'then',
      repeat1($.case_statement),
    )),

    case_statement: $ => seq(
      field('pattern', $.expression),
      ':',
      field('body', choice($.block, $.call_expression)),
    ),

    // Loop statement (multiple variants)
    loop_statement: $ => seq(
      'loop',
      optional(':'),
      choice(
        // Range loop: loop:start to end
        seq(
          field('start', $.expression),
          'to',
          field('end', $.expression),
        ),
        // While loop: loop:condition
        field('condition', $.expression),
        // Array loop: loop element in array
        seq(
          field('element', $.identifier),
          'in',
          field('iterable', $.expression),
        ),
        // Dict loop: loop key,value in dict
        seq(
          field('key', $.identifier),
          ',',
          field('value', $.identifier),
          'in',
          field('iterable', $.expression),
        ),
      ),
      'then',
      field('body', $.block),
    ),

    // When statement (compile-time conditional)
    when_statement: $ => seq(
      'when',
      field('condition', $.identifier),
      'then',
      field('body', $.block),
    ),

    // Return statement
    return_statement: $ => prec.right(seq(
      'return',
      optional(field('value', $.expression)),
    )),

    // Block
    block: $ => prec.right(seq(
      repeat(choice(';', '\n')),
      $._statement,
      repeat(seq(
        repeat1(choice(';', '\n')),
        $._statement,
      )),
      repeat(choice(';', '\n')),
    )),

    // Expressions
    expression: $ => choice(
      $.binary_expression,
      $.unary_expression,
      $.call_expression,
      $.array_access,
      $.dict_access,
      $.array_literal,
      $.dict_literal,
      $.identifier,
      $.number,
      $.string,
      $.char,
      $.boolean,
      $.parenthesized_expression,
    ),

    // Binary expressions with precedence
    binary_expression: $ => choice(
      // Logical OR
      prec.left(1, seq(
        field('left', $.expression),
        field('operator', 'or'),
        field('right', $.expression),
      )),
      // Logical AND
      prec.left(2, seq(
        field('left', $.expression),
        field('operator', 'and'),
        field('right', $.expression),
      )),
      // Comparison
      prec.left(3, seq(
        field('left', $.expression),
        field('operator', choice(
          'is',
          '>',
          '<',
          '>=',
          '<=',
          'greater_than',
          'lesser_than',
        )),
        field('right', $.expression),
      )),
      // Addition/Subtraction
      prec.left(4, seq(
        field('left', $.expression),
        field('operator', choice('+', '-', 'plus', 'minus')),
        field('right', $.expression),
      )),
      // Multiplication/Division/Modulo
      prec.left(5, seq(
        field('left', $.expression),
        field('operator', choice('*', '/', '%', 'times', 'div', 'mod')),
        field('right', $.expression),
      )),
    ),

    // Unary expressions
    unary_expression: $ => prec.right(6, seq(
      field('operator', choice('not', '-')),
      field('operand', $.expression),
    )),

    // Function call
    call_expression: $ => prec(7, seq(
      field('function', $.identifier),
      '|',
      optional($.argument_list),
      '|',
    )),

    argument_list: $ => seq(
      $.expression,
      repeat(seq(',', $.expression)),
    ),

    // Array access
    array_access: $ => prec(8, seq(
      field('array', choice($.identifier, $.call_expression)),
      '<',
      field('index', $.expression),
      '>',
    )),

    // Dict access
    dict_access: $ => prec(8, seq(
      field('dict', choice($.identifier, $.call_expression)),
      '{',
      field('key', $.string),
      '}',
    )),

    // Array literal
    array_literal: $ => seq(
      '<',
      optional(seq(
        $.expression,
        repeat(seq(',', $.expression)),
      )),
      '>',
    ),

    // Dict literal
    dict_literal: $ => seq(
      '{',
      optional(seq(
        $.dict_pair,
        repeat(seq(',', $.dict_pair)),
      )),
      '}',
    ),

    dict_pair: $ => seq(
      field('key', $.string),
      ':',
      field('value', $.expression),
    ),

    // Parenthesized expression
    parenthesized_expression: $ => seq(
      '(',
      $.expression,
      ')',
    ),

    // Primitives
    identifier: $ => /[a-zA-Z_][a-zA-Z0-9_]*/,

    number: $ => choice(
      /\d+\.\d+/,  // float
      /\d+/,       // int
    ),

    string: $ => token(seq(
      '"',
      repeat(choice(
        /[^"\\]/,
        /\\./,
      )),
      '"',
    )),

    char: $ => token(seq(
      "'",
      choice(
        /[^'\\]/,
        /\\./,
      ),
      "'",
    )),

    boolean: $ => choice(
      'true',
      'false',
    ),
  },
});
