       IDENTIFICATION DIVISION.
       PROGRAM-ID. PROMO-CODE-GENERATOR.
       
       ENVIRONMENT DIVISION.
       
       DATA DIVISION.
       WORKING-STORAGE SECTION.
       01 WS-INPUT.
          05 WS-CODE-COUNT   PIC 9(2) VALUE 5.
       
       01 WS-LOOP-COUNTER    PIC 9(2).
       
       01 WS-CURRENT-DATETIME.
          05 WS-DATE.
             10 WS-YEAR    PIC 9(4).
             10 WS-MONTH   PIC 9(2).
             10 WS-DAY     PIC 9(2).
          05 WS-TIME.
             10 WS-HOUR    PIC 9(2).
             10 WS-MINUTE  PIC 9(2).
             10 WS-SECOND  PIC 9(2).
             10 FILLER     PIC 9(2).
       
       01 WS-PROMO-CODE.
          05 WS-PREFIX     PIC X(3) VALUE "PRO".
          05 WS-RANDOM     PIC 9(6).
       
       01 WS-DISCOUNT-TYPES.
          05 WS-DISCOUNT-1 PIC X(15) VALUE "10% OFF".
          05 WS-DISCOUNT-2 PIC X(15) VALUE "20% OFF".
          05 WS-DISCOUNT-3 PIC X(15) VALUE "30% OFF".
       
       01 WS-SELECTED-DISCOUNT PIC X(15).
       
       PROCEDURE DIVISION.
           ACCEPT WS-CODE-COUNT FROM COMMAND-LINE
           
           MOVE FUNCTION CURRENT-DATE TO WS-CURRENT-DATETIME
           
           PERFORM VARYING WS-LOOP-COUNTER FROM 1 BY 1 
               UNTIL WS-LOOP-COUNTER > WS-CODE-COUNT
               
               COMPUTE WS-RANDOM = 
                   FUNCTION RANDOM * 899999 + 100000
               
               EVALUATE WS-SECOND
                 WHEN 0 THRU 19 
                   MOVE WS-DISCOUNT-1 TO WS-SELECTED-DISCOUNT
                 WHEN 20 THRU 39 
                   MOVE WS-DISCOUNT-2 TO WS-SELECTED-DISCOUNT
                 WHEN 40 THRU 59 
                   MOVE WS-DISCOUNT-3 TO WS-SELECTED-DISCOUNT
                 WHEN OTHER 
                   MOVE WS-DISCOUNT-1 TO WS-SELECTED-DISCOUNT
               END-EVALUATE
               
               STRING 
                 WS-PREFIX 
                 WS-RANDOM 
                 DELIMITED BY SIZE 
                 INTO WS-PROMO-CODE
               
               DISPLAY WS-PROMO-CODE " - " WS-SELECTED-DISCOUNT
           END-PERFORM
           
           STOP RUN.