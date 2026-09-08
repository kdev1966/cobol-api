       IDENTIFICATION DIVISION.
       PROGRAM-ID. PROMO-CODE-GENERATOR.

      *> Formate des codes promotionnels a partir d'entropie fournie par
      *> l'appelant. La generation aleatoire est deleguee a l'API, seule
      *> a disposer d'un generateur cryptographique ; ce programme porte
      *> les regles metier : forme du code et repartition des paliers.
      *>
      *> Usage  : promo_generator <nombre-de-codes>
      *> Entree : <nombre-de-codes> lignes de 13 caracteres sur stdin,
      *>          soit 10 caracteres de corps de code puis 3 chiffres
      *>          designant le palier de remise
      *> Sortie : une ligne "<CODE> - <REMISE>" par code

       ENVIRONMENT DIVISION.

       DATA DIVISION.
       WORKING-STORAGE SECTION.

       01 WS-ARG-COUNT          PIC 9(4)  VALUE 0.
       01 WS-COUNT-ARG          PIC X(10) VALUE SPACES.
       01 WS-CODE-COUNT         PIC 9(4)  VALUE 0.
       01 WS-LOOP-COUNTER       PIC 9(4)  VALUE 0.

       01 WS-ENTROPY            PIC X(13) VALUE SPACES.
       01 WS-ENTROPY-FIELDS REDEFINES WS-ENTROPY.
          05 WS-CODE-BODY       PIC X(10).
          05 WS-TIER-SOURCE     PIC 9(3).

       01 WS-TIER-INDEX         PIC 9     VALUE 1.

       01 WS-DISCOUNT-TABLE.
          05 FILLER             PIC X(15) VALUE "10% OFF".
          05 FILLER             PIC X(15) VALUE "20% OFF".
          05 FILLER             PIC X(15) VALUE "30% OFF".
       01 WS-DISCOUNT-TIERS REDEFINES WS-DISCOUNT-TABLE.
          05 WS-DISCOUNT        PIC X(15) OCCURS 3 TIMES.

       01 WS-PROMO-CODE.
          05 WS-PREFIX          PIC X(3)  VALUE "PRO".
          05 WS-BODY            PIC X(10) VALUE SPACES.

       PROCEDURE DIVISION.

       MAIN-PROCEDURE.
           PERFORM READ-CODE-COUNT
           PERFORM VARYING WS-LOOP-COUNTER FROM 1 BY 1
               UNTIL WS-LOOP-COUNTER > WS-CODE-COUNT
               PERFORM EMIT-ONE-CODE
           END-PERFORM
           STOP RUN.

      *> Lit le nombre de codes demande sur la ligne de commande.
      *> NUMVAL est utilise plutot qu'un ACCEPT direct dans un champ
      *> numerique, qui cadrerait la chaine a gauche et fausserait la
      *> valeur.
       READ-CODE-COUNT.
           ACCEPT WS-ARG-COUNT FROM ARGUMENT-NUMBER
           IF WS-ARG-COUNT < 1
               DISPLAY "usage: promo_generator <nombre-de-codes>"
                   UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF

           ACCEPT WS-COUNT-ARG FROM ARGUMENT-VALUE
           COMPUTE WS-CODE-COUNT = FUNCTION NUMVAL(WS-COUNT-ARG)

           IF WS-CODE-COUNT < 1
               DISPLAY "nombre de codes invalide: "
                   FUNCTION TRIM(WS-COUNT-ARG) UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF.

      *> Consomme une ligne d'entropie et emet le code correspondant.
       EMIT-ONE-CODE.
           MOVE SPACES TO WS-ENTROPY
           ACCEPT WS-ENTROPY

           IF WS-CODE-BODY = SPACES OR WS-TIER-SOURCE IS NOT NUMERIC
               DISPLAY "entropie invalide a la ligne "
                   WS-LOOP-COUNTER UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           MOVE WS-CODE-BODY TO WS-BODY
           COMPUTE WS-TIER-INDEX = FUNCTION MOD(WS-TIER-SOURCE, 3) + 1

           DISPLAY WS-PROMO-CODE " - "
               FUNCTION TRIM(WS-DISCOUNT(WS-TIER-INDEX)).
