       IDENTIFICATION DIVISION.
       PROGRAM-ID. LOAN-CAPACITY.

      *> Calcul inverse : a partir d'une mensualite supportable, le capital
      *> maximal empruntable.
      *>
      *> La formule fermee existe pour chaque methode, mais elle ignore les
      *> arrondis : le capital qu'elle rend produit parfois une premiere
      *> mensualite d'un millime au-dessus du budget. On cherche donc par
      *> dichotomie le plus grand capital dont la premiere mensualite tient
      *> dans le budget, ce qui est exact par construction et vaut pour les
      *> trois methodes comme pour les deux assiettes d'assurance.
      *>
      *> Les montants sont en millimes.
      *>
      *> Avec un differe, la contrainte de budget porte sur la premiere
      *> echeance amortissante et non sur la franchise : celle-ci ne paie que
      *> les interets, et s'y fier donnerait une capacite follement
      *> optimiste. Le capital reste intact pendant la franchise, la
      *> mensualite se calcule donc sur les seules echeances amortissantes.
      *>
      *> Entree : une ligne de 39 caracteres sur stdin
      *>            mensualite max  9(11)V999  positions  1-14
      *>            taux annuel     9(2)V9(6)  positions 15-22
      *>            duree mois      9(4)       positions 23-26
      *>            taux assurance  9(2)V9(6)  positions 27-34
      *>            differe mois    9(3)       positions 35-37
      *>            methode         X          position     38
      *>            assiette assur. X          position     39
      *>
      *> Sortie : un enregistrement "C" de 33 caracteres
      *>            capital         9(11)V999  positions  2-15
      *>            mensualite      9(11)V999  positions 16-29
      *>              obtenue, toujours inferieure ou egale au budget
      *>            marge           9(4)       positions 30-33
      *>              millimes non employes du budget
      *>
      *> Retour : 0 succes, 2 entree malformee, 3 parametres hors bornes,
      *>          4 methode ou assiette inconnue, 5 differe incompatible
      *>          avec la duree.

       ENVIRONMENT DIVISION.

       DATA DIVISION.
       WORKING-STORAGE SECTION.

       01 WS-ENTREE             PIC X(39) VALUE SPACES.
       01 WS-ENTREE-CHAMPS REDEFINES WS-ENTREE.
          05 WS-E-CHIFFRES      PIC X(37).
          05 WS-E-METHODE       PIC X.
          05 WS-E-ASSIETTE      PIC X.
       01 WS-E-DETAIL REDEFINES WS-ENTREE.
          05 WS-E-BUDGET        PIC 9(11)V999.
          05 WS-E-TAUX          PIC 9(2)V9(6).
          05 WS-E-DUREE         PIC 9(4).
          05 WS-E-TAUX-ASSUR    PIC 9(2)V9(6).
          05 WS-E-DIFFERE       PIC 9(3).
          05 FILLER             PIC X(2).

       78 METHODE-ANNUITE       VALUE "A".
       78 METHODE-CAPITAL       VALUE "C".
       78 METHODE-IN-FINE       VALUE "I".
       78 ASSIETTE-AUCUNE       VALUE "N".
       78 ASSIETTE-INITIAL      VALUE "I".
       78 ASSIETTE-RESTANT      VALUE "R".
       78 DUREE-MAX             VALUE 600.
      *> Cinquante tours ramenent un encadrement de cent milliards de dinars
      *> a moins d'un millime : 1e14 / 2^50 vaut environ 0,09.
       78 ITERATIONS            VALUE 50.

       01 WS-TAUX-MENSUEL       PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-ASSUR-MENSUEL PIC 9V9(18)    VALUE 0.
       01 WS-FACTEUR            PIC 9(9)V9(18) VALUE 0.
      *> Echeances effectivement amortissantes, la franchise deduite.
       01 WS-DUREE-AMORT        PIC 9(4)       VALUE 0.

       01 WS-BAS                PIC 9(11)V999  VALUE 0.
       01 WS-HAUT               PIC 9(11)V999  VALUE 0.
       01 WS-ESSAI              PIC 9(11)V999  VALUE 0.
       01 WS-ITER               PIC 9(3)       VALUE 0.

       01 WS-INTERET            PIC 9(11)V999  VALUE 0.
       01 WS-PART-CAPITAL       PIC 9(11)V999  VALUE 0.
       01 WS-ASSURANCE          PIC 9(11)V999  VALUE 0.
       01 WS-MENSUALITE         PIC 9(11)V999  VALUE 0.

       01 WS-CAPITAL-RETENU     PIC 9(11)V999  VALUE 0.
       01 WS-MENSUALITE-RETENUE PIC 9(11)V999  VALUE 0.
       01 WS-MARGE              PIC 9(4)       VALUE 0.

       01 WS-SORTIE.
          05 FILLER             PIC X          VALUE "C".
          05 WS-S-CAPITAL       PIC 9(11)V999  VALUE 0.
          05 WS-S-MENSUALITE    PIC 9(11)V999  VALUE 0.
          05 WS-S-MARGE         PIC 9(4)       VALUE 0.

       PROCEDURE DIVISION.

       MAIN-PROCEDURE.
           PERFORM LIRE-DEMANDE
           PERFORM PREPARER-CALCUL
           PERFORM CHERCHER-CAPITAL
           PERFORM ECRIRE-RESULTAT
           STOP RUN.

       LIRE-DEMANDE.
           ACCEPT WS-ENTREE

           IF WS-E-CHIFFRES IS NOT NUMERIC
               DISPLAY "entree malformee : 34 chiffres puis deux lettres"
                   UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-BUDGET = 0 OR WS-E-DUREE = 0
               DISPLAY "mensualite et duree doivent etre non nulles"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-DUREE > DUREE-MAX
               DISPLAY "duree superieure a " DUREE-MAX " mois"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-DIFFERE >= WS-E-DUREE
               DISPLAY "le differe doit laisser au moins une echeance "
                   "amortissante" UPON SYSERR
               MOVE 5 TO RETURN-CODE
               STOP RUN
           END-IF

           COMPUTE WS-DUREE-AMORT = WS-E-DUREE - WS-E-DIFFERE

           IF WS-E-METHODE NOT = METHODE-ANNUITE
              AND WS-E-METHODE NOT = METHODE-CAPITAL
              AND WS-E-METHODE NOT = METHODE-IN-FINE
               DISPLAY "methode inconnue : " WS-E-METHODE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-ASSIETTE NOT = ASSIETTE-AUCUNE
              AND WS-E-ASSIETTE NOT = ASSIETTE-INITIAL
              AND WS-E-ASSIETTE NOT = ASSIETTE-RESTANT
               DISPLAY "assiette d'assurance inconnue : "
                   WS-E-ASSIETTE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF.

       PREPARER-CALCUL.
           COMPUTE WS-TAUX-MENSUEL = WS-E-TAUX / 100 / 12
           COMPUTE WS-TAUX-ASSUR-MENSUEL = WS-E-TAUX-ASSUR / 100 / 12

           IF WS-TAUX-MENSUEL NOT = 0
               COMPUTE WS-FACTEUR =
                   (1 + WS-TAUX-MENSUEL) ** WS-DUREE-AMORT
           END-IF.

      *> La premiere mensualite croit avec le capital. La dichotomie retient
      *> donc le plus grand capital qui tient dans le budget.
       CHERCHER-CAPITAL.
           MOVE 0 TO WS-BAS
           MOVE 99999999999.999 TO WS-HAUT
           MOVE 0 TO WS-CAPITAL-RETENU
           MOVE 0 TO WS-MENSUALITE-RETENUE

           PERFORM VARYING WS-ITER FROM 1 BY 1 UNTIL WS-ITER > ITERATIONS
               COMPUTE WS-ESSAI ROUNDED = (WS-BAS + WS-HAUT) / 2
               PERFORM PREMIERE-MENSUALITE

               IF WS-MENSUALITE > WS-E-BUDGET
                   MOVE WS-ESSAI TO WS-HAUT
               ELSE
                   MOVE WS-ESSAI TO WS-BAS
                   MOVE WS-ESSAI TO WS-CAPITAL-RETENU
                   MOVE WS-MENSUALITE TO WS-MENSUALITE-RETENUE
               END-IF
           END-PERFORM

           COMPUTE WS-MARGE =
               (WS-E-BUDGET - WS-MENSUALITE-RETENUE) * 1000.

      *> Premiere mensualite amortissante pour le capital d'essai, arrondis
      *> compris : c'est elle qui doit tenir dans le budget, et non une valeur
      *> theorique ni celle de la franchise.
       PREMIERE-MENSUALITE.
           COMPUTE WS-INTERET ROUNDED = WS-ESSAI * WS-TAUX-MENSUEL

           EVALUATE WS-E-METHODE
               WHEN METHODE-ANNUITE
                   IF WS-TAUX-MENSUEL = 0
                       COMPUTE WS-PART-CAPITAL ROUNDED =
                           WS-ESSAI / WS-DUREE-AMORT
                   ELSE
      *> Part de capital de la premiere echeance : la mensualite
      *> constante moins les interets de la periode.
                       COMPUTE WS-PART-CAPITAL ROUNDED =
                           WS-ESSAI * WS-TAUX-MENSUEL
                           / (1 - (1 / WS-FACTEUR))
                           - WS-INTERET
                   END-IF
               WHEN METHODE-CAPITAL
                   COMPUTE WS-PART-CAPITAL ROUNDED =
                       WS-ESSAI / WS-DUREE-AMORT
               WHEN METHODE-IN-FINE
                   MOVE 0 TO WS-PART-CAPITAL
           END-EVALUATE

      *> Sur la premiere echeance, les deux assiettes coincident : le capital
      *> restant du y vaut encore le capital initial.
           IF WS-E-ASSIETTE = ASSIETTE-AUCUNE
               MOVE 0 TO WS-ASSURANCE
           ELSE
               COMPUTE WS-ASSURANCE ROUNDED =
                   WS-ESSAI * WS-TAUX-ASSUR-MENSUEL
           END-IF

           COMPUTE WS-MENSUALITE =
               WS-PART-CAPITAL + WS-INTERET + WS-ASSURANCE.

       ECRIRE-RESULTAT.
           MOVE WS-CAPITAL-RETENU     TO WS-S-CAPITAL
           MOVE WS-MENSUALITE-RETENUE TO WS-S-MENSUALITE
           MOVE WS-MARGE              TO WS-S-MARGE
           DISPLAY WS-SORTIE.
