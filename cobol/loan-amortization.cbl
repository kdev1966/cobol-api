       IDENTIFICATION DIVISION.
       PROGRAM-ID. LOAN-AMORTIZATION.

      *> Produit l'echeancier d'un pret a mensualite constante.
      *>
      *> L'arithmetique est en decimal a virgule fixe : les montants sont
      *> exacts au centime, ce qu'un flottant binaire ne garantit pas.
      *> Trois invariants sont tenus par construction :
      *>   1. la somme des parts de capital egale le capital emprunte
      *>   2. la somme des echeances egale capital + interets
      *>   3. le solde apres la derniere echeance est nul
      *> La derniere echeance absorbe le residu d'arrondi, comme le veut
      *> la pratique bancaire.
      *>
      *> Entree : une ligne de 25 chiffres sur stdin
      *>            capital     9(11)V99   positions  1-13
      *>            taux annuel 9(2)V9(6)  positions 14-21
      *>            duree mois  9(4)       positions 22-25
      *> Sortie : enregistrements a largeur fixe, un par ligne.
      *>          "R" recapitulatif : echeances 9(4), mensualite 9(11)V99,
      *>              total interets 9(13)V99, total du 9(13)V99   -> 48 car.
      *>          "E" echeance      : numero 9(4), paiement 9(11)V99,
      *>              interets, capital, solde, tous 9(11)V99      -> 57 car.
      *>          Le recapitulatif precede les echeances, dans l'ordre.
      *>
      *>          JSON GENERATE n'est deliberement pas utilise : le paquet
      *>          GnuCOBOL des distributions est construit sans bibliotheque
      *>          JSON, et l'instruction compile alors sans rien produire, ni
      *>          erreur ni avertissement. La mise en forme revient donc a
      *>          l'appelant.
      *> Retour : 0 succes, 2 entree malformee, 3 parametres hors bornes.

       ENVIRONMENT DIVISION.

       DATA DIVISION.
       WORKING-STORAGE SECTION.

       01 WS-ENTREE             PIC X(25) VALUE SPACES.
       01 WS-ENTREE-CHAMPS REDEFINES WS-ENTREE.
          05 WS-E-CAPITAL       PIC 9(11)V99.
          05 WS-E-TAUX          PIC 9(2)V9(6).
          05 WS-E-DUREE         PIC 9(4).

       01 WS-TAUX-MENSUEL       PIC 9V9(18)    VALUE 0.
       01 WS-FACTEUR            PIC 9(9)V9(18) VALUE 0.
       01 WS-MENSUALITE         PIC 9(11)V99   VALUE 0.

       01 WS-I                  PIC 9(4)       VALUE 0.
       01 WS-SOLDE              PIC S9(11)V99  VALUE 0.
       01 WS-INTERET            PIC 9(11)V99   VALUE 0.
       01 WS-PART-CAPITAL       PIC 9(11)V99   VALUE 0.
       01 WS-ECHEANCE           PIC 9(11)V99   VALUE 0.

       01 WS-CUM-INTERET        PIC 9(13)V99   VALUE 0.
       01 WS-CUM-ECHEANCE       PIC 9(13)V99   VALUE 0.

      *> Le deroulement sert deux fois : une passe muette pour totaliser,
      *> une passe emettrice. Un seul corps de boucle, donc une seule
      *> regle de calcul.
       01 WS-EMETTRE            PIC X          VALUE "N".

       01 WS-RECAP.
          05 FILLER             PIC X          VALUE "R".
          05 WS-R-ECHEANCES     PIC 9(4)       VALUE 0.
          05 WS-R-MENSUALITE    PIC 9(11)V99   VALUE 0.
          05 WS-R-INTERETS      PIC 9(13)V99   VALUE 0.
          05 WS-R-TOTAL         PIC 9(13)V99   VALUE 0.

       01 WS-LIGNE.
          05 FILLER             PIC X          VALUE "E".
          05 WS-L-NUMERO        PIC 9(4)       VALUE 0.
          05 WS-L-PAIEMENT      PIC 9(11)V99   VALUE 0.
          05 WS-L-INTERETS      PIC 9(11)V99   VALUE 0.
          05 WS-L-CAPITAL       PIC 9(11)V99   VALUE 0.
          05 WS-L-SOLDE         PIC 9(11)V99   VALUE 0.

       PROCEDURE DIVISION.

       MAIN-PROCEDURE.
           PERFORM LIRE-DEMANDE
           PERFORM CALCULER-MENSUALITE

           MOVE "N" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER
           PERFORM ECRIRE-RECAPITULATIF

           MOVE "O" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER

           STOP RUN.

       LIRE-DEMANDE.
           ACCEPT WS-ENTREE

           IF WS-ENTREE IS NOT NUMERIC
               DISPLAY "entree malformee : 25 chiffres attendus"
                   UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-CAPITAL = 0 OR WS-E-DUREE = 0
               DISPLAY "capital et duree doivent etre non nuls"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF.

      *> Taux periodique proportionnel : taux nominal annuel divise par
      *> douze. C'est la convention du taux nominal, distincte du taux
      *> actuariel equivalent.
       CALCULER-MENSUALITE.
           COMPUTE WS-TAUX-MENSUEL = WS-E-TAUX / 100 / 12

           IF WS-TAUX-MENSUEL = 0
      *> Sans interets la formule diviserait par zero.
               COMPUTE WS-MENSUALITE ROUNDED =
                   WS-E-CAPITAL / WS-E-DUREE
           ELSE
               COMPUTE WS-FACTEUR =
                   (1 + WS-TAUX-MENSUEL) ** WS-E-DUREE
               COMPUTE WS-MENSUALITE ROUNDED =
                   WS-E-CAPITAL * WS-TAUX-MENSUEL
                   / (1 - (1 / WS-FACTEUR))
           END-IF.

       DEROULER-ECHEANCIER.
           MOVE WS-E-CAPITAL TO WS-SOLDE
           MOVE 0 TO WS-CUM-INTERET
           MOVE 0 TO WS-CUM-ECHEANCE

           PERFORM VARYING WS-I FROM 1 BY 1
               UNTIL WS-I > WS-E-DUREE

               COMPUTE WS-INTERET ROUNDED =
                   WS-SOLDE * WS-TAUX-MENSUEL

               IF WS-I = WS-E-DUREE
      *> La derniere echeance solde le capital restant, quel que soit
      *> le residu accumule par les arrondis mensuels.
                   MOVE WS-SOLDE TO WS-PART-CAPITAL
                   COMPUTE WS-ECHEANCE =
                       WS-PART-CAPITAL + WS-INTERET
               ELSE
                   MOVE WS-MENSUALITE TO WS-ECHEANCE
                   COMPUTE WS-PART-CAPITAL =
                       WS-ECHEANCE - WS-INTERET
               END-IF

               SUBTRACT WS-PART-CAPITAL FROM WS-SOLDE
               ADD WS-INTERET  TO WS-CUM-INTERET
               ADD WS-ECHEANCE TO WS-CUM-ECHEANCE

               IF WS-EMETTRE = "O"
                   PERFORM ECRIRE-LIGNE
               END-IF

           END-PERFORM.

       ECRIRE-RECAPITULATIF.
           MOVE WS-E-DUREE      TO WS-R-ECHEANCES
           MOVE WS-MENSUALITE   TO WS-R-MENSUALITE
           MOVE WS-CUM-INTERET  TO WS-R-INTERETS
           MOVE WS-CUM-ECHEANCE TO WS-R-TOTAL

           DISPLAY WS-RECAP.

       ECRIRE-LIGNE.
           MOVE WS-I            TO WS-L-NUMERO
           MOVE WS-ECHEANCE     TO WS-L-PAIEMENT
           MOVE WS-INTERET      TO WS-L-INTERETS
           MOVE WS-PART-CAPITAL TO WS-L-CAPITAL
           MOVE WS-SOLDE        TO WS-L-SOLDE

           DISPLAY WS-LIGNE.
