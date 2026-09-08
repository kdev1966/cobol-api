       IDENTIFICATION DIVISION.
       PROGRAM-ID. LOAN-AMORTIZATION.

      *> Produit l'echeancier d'un pret, selon trois methodes.
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
      *> Methodes :
      *>   A  annuite constante : l'echeance ne bouge pas, la part de
      *>      capital croit a mesure que les interets diminuent
      *>   C  capital constant  : la part de capital ne bouge pas,
      *>      l'echeance decroit
      *>   I  in fine           : interets seuls, capital rembourse en
      *>      totalite a la derniere echeance
      *>
      *> Entree : une ligne de 26 caracteres sur stdin
      *>            capital     9(11)V99   positions  1-13
      *>            taux annuel 9(2)V9(6)  positions 14-21
      *>            duree mois  9(4)       positions 22-25
      *>            methode     X          position     26
      *> Sortie : enregistrements a largeur fixe, un par ligne.
      *>          "R" recapitulatif : echeances 9(4), premiere echeance
      *>              9(11)V99, derniere echeance 9(11)V99, total interets
      *>              9(13)V99, total du 9(13)V99, taeg 9(2)V9(4)  -> 67 car.
      *>          "E" echeance      : numero 9(4), paiement 9(11)V99,
      *>              interets, capital, solde, tous 9(11)V99      -> 57 car.
      *>          Le recapitulatif precede les echeances, dans l'ordre.
      *>
      *>          JSON GENERATE n'est deliberement pas utilise : le paquet
      *>          GnuCOBOL des distributions est construit sans bibliotheque
      *>          JSON, et l'instruction compile alors sans rien produire, ni
      *>          erreur ni avertissement. La mise en forme revient donc a
      *>          l'appelant.
      *> Retour : 0 succes, 2 entree malformee, 3 parametres hors bornes,
      *>          4 methode inconnue.
      *>
      *> Le TAEG est le taux actuariel annuel qui egalise la valeur actuelle
      *> des echeances au capital emprunte. Comme les echeances sont arrondies
      *> au centime, il ne se deduit pas du taux nominal : il faut le
      *> resoudre. C'est fait par dichotomie sur le taux periodique, en
      *> arithmetique decimale exacte.

       ENVIRONMENT DIVISION.

       DATA DIVISION.
       WORKING-STORAGE SECTION.

       01 WS-ENTREE             PIC X(26) VALUE SPACES.
       01 WS-ENTREE-CHAMPS REDEFINES WS-ENTREE.
          05 WS-E-CHIFFRES      PIC X(25).
          05 WS-E-METHODE       PIC X.
       01 WS-E-DETAIL REDEFINES WS-ENTREE.
          05 WS-E-CAPITAL       PIC 9(11)V99.
          05 WS-E-TAUX          PIC 9(2)V9(6).
          05 WS-E-DUREE         PIC 9(4).
          05 FILLER             PIC X.

       78 METHODE-ANNUITE       VALUE "A".
       78 METHODE-CAPITAL       VALUE "C".
       78 METHODE-IN-FINE       VALUE "I".

       01 WS-TAUX-MENSUEL       PIC 9V9(18)    VALUE 0.
       01 WS-FACTEUR            PIC 9(9)V9(18) VALUE 0.
       01 WS-MENSUALITE         PIC 9(11)V99   VALUE 0.
      *> Part de capital fixe de la methode a capital constant.
       01 WS-AMORT-FIXE         PIC 9(11)V99   VALUE 0.

       01 WS-I                  PIC 9(4)       VALUE 0.
       01 WS-SOLDE              PIC S9(11)V99  VALUE 0.
       01 WS-INTERET            PIC 9(11)V99   VALUE 0.
       01 WS-PART-CAPITAL       PIC 9(11)V99   VALUE 0.
       01 WS-ECHEANCE           PIC 9(11)V99   VALUE 0.

       01 WS-CUM-INTERET        PIC 9(13)V99   VALUE 0.
       01 WS-CUM-ECHEANCE       PIC 9(13)V99   VALUE 0.

      *> Duree maximale acceptee, qui dimensionne la table des paiements
      *> conservee pour la resolution du TAEG.
       78 DUREE-MAX             VALUE 600.
       78 ITERATIONS-TAEG       VALUE 40.

       01 WS-PAIEMENTS.
          05 WS-PAIEMENT        PIC 9(11)V99 OCCURS 600 TIMES.

       01 WS-J                  PIC 9(4)       VALUE 0.
       01 WS-ITER               PIC 9(3)       VALUE 0.
       01 WS-TAUX-BAS           PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-HAUT          PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-ESSAI         PIC 9V9(18)    VALUE 0.
       01 WS-ESCOMPTE           PIC 9V9(18)    VALUE 0.
       01 WS-VALEUR-ACTUELLE    PIC 9(13)V9(6) VALUE 0.
       01 WS-TAEG               PIC 9(2)V9(4)  VALUE 0.

      *> Le deroulement sert deux fois : une passe muette pour totaliser,
      *> une passe emettrice. Un seul corps de boucle, donc une seule
      *> regle de calcul.
       01 WS-EMETTRE            PIC X          VALUE "N".

       01 WS-PREMIERE            PIC 9(11)V99   VALUE 0.
       01 WS-DERNIERE            PIC 9(11)V99   VALUE 0.

       01 WS-RECAP.
          05 FILLER             PIC X          VALUE "R".
          05 WS-R-ECHEANCES     PIC 9(4)       VALUE 0.
          05 WS-R-PREMIERE      PIC 9(11)V99   VALUE 0.
          05 WS-R-DERNIERE      PIC 9(11)V99   VALUE 0.
          05 WS-R-INTERETS      PIC 9(13)V99   VALUE 0.
          05 WS-R-TOTAL         PIC 9(13)V99   VALUE 0.
          05 WS-R-TAEG          PIC 9(2)V9(4)  VALUE 0.

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
           PERFORM CALCULER-TAEG
           PERFORM ECRIRE-RECAPITULATIF

           MOVE "O" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER

           STOP RUN.

       LIRE-DEMANDE.
           ACCEPT WS-ENTREE

           IF WS-E-CHIFFRES IS NOT NUMERIC
               DISPLAY "entree malformee : 25 chiffres puis la methode"
                   UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-METHODE NOT = METHODE-ANNUITE
              AND WS-E-METHODE NOT = METHODE-CAPITAL
              AND WS-E-METHODE NOT = METHODE-IN-FINE
               DISPLAY "methode inconnue : " WS-E-METHODE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-CAPITAL = 0 OR WS-E-DUREE = 0
               DISPLAY "capital et duree doivent etre non nuls"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-DUREE > DUREE-MAX
               DISPLAY "duree superieure a " DUREE-MAX " mois"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF.

      *> Taux periodique proportionnel : taux nominal annuel divise par
      *> douze. C'est la convention du taux nominal, distincte du taux
      *> actuariel equivalent.
       CALCULER-MENSUALITE.
           COMPUTE WS-TAUX-MENSUEL = WS-E-TAUX / 100 / 12

           EVALUATE WS-E-METHODE
               WHEN METHODE-ANNUITE
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
                   END-IF

               WHEN METHODE-CAPITAL
                   COMPUTE WS-AMORT-FIXE ROUNDED =
                       WS-E-CAPITAL / WS-E-DUREE

               WHEN METHODE-IN-FINE
      *> Aucun capital n'est rembourse avant la derniere echeance.
                   MOVE 0 TO WS-AMORT-FIXE
           END-EVALUATE.

       DEROULER-ECHEANCIER.
           MOVE WS-E-CAPITAL TO WS-SOLDE
           MOVE 0 TO WS-CUM-INTERET
           MOVE 0 TO WS-CUM-ECHEANCE

           PERFORM VARYING WS-I FROM 1 BY 1
               UNTIL WS-I > WS-E-DUREE

               COMPUTE WS-INTERET ROUNDED =
                   WS-SOLDE * WS-TAUX-MENSUEL

               IF WS-I = WS-E-DUREE
      *> Quelle que soit la methode, la derniere echeance solde le
      *> capital restant : c'est elle qui absorbe le residu accumule
      *> par les arrondis mensuels.
                   MOVE WS-SOLDE TO WS-PART-CAPITAL
               ELSE
                   EVALUATE WS-E-METHODE
                       WHEN METHODE-ANNUITE
                           COMPUTE WS-PART-CAPITAL =
                               WS-MENSUALITE - WS-INTERET
                       WHEN METHODE-CAPITAL
                           MOVE WS-AMORT-FIXE TO WS-PART-CAPITAL
                       WHEN METHODE-IN-FINE
                           MOVE 0 TO WS-PART-CAPITAL
                   END-EVALUATE
               END-IF

               COMPUTE WS-ECHEANCE = WS-PART-CAPITAL + WS-INTERET

               SUBTRACT WS-PART-CAPITAL FROM WS-SOLDE
               ADD WS-INTERET  TO WS-CUM-INTERET
               ADD WS-ECHEANCE TO WS-CUM-ECHEANCE

               MOVE WS-ECHEANCE TO WS-PAIEMENT(WS-I)

               IF WS-I = 1
                   MOVE WS-ECHEANCE TO WS-PREMIERE
               END-IF
               IF WS-I = WS-E-DUREE
                   MOVE WS-ECHEANCE TO WS-DERNIERE
               END-IF

               IF WS-EMETTRE = "O"
                   PERFORM ECRIRE-LIGNE
               END-IF

           END-PERFORM.

      *> Dichotomie sur le taux periodique : la valeur actuelle des
      *> echeances decroit quand le taux monte, l'encadrement se resserre
      *> donc de moitie a chaque tour.
      *>
      *> Les echeances etant construites a partir du taux nominal, le taux
      *> recherche en est tres proche ; 0,25 par mois le majore largement,
      *> le taux annuel accepte plafonnant a 99,999999 %, soit 0,0833 par
      *> mois. Quarante tours ramenent alors l'incertitude a 2,3e-13, bien
      *> au-dela des quatre decimales rendues.
       CALCULER-TAEG.
           MOVE 0 TO WS-TAUX-BAS
           MOVE 0.25 TO WS-TAUX-HAUT

           PERFORM VARYING WS-ITER FROM 1 BY 1
               UNTIL WS-ITER > ITERATIONS-TAEG
               COMPUTE WS-TAUX-ESSAI =
                   (WS-TAUX-BAS + WS-TAUX-HAUT) / 2
               PERFORM VALEUR-ACTUELLE-DES-ECHEANCES
               IF WS-VALEUR-ACTUELLE > WS-E-CAPITAL
                   MOVE WS-TAUX-ESSAI TO WS-TAUX-BAS
               ELSE
                   MOVE WS-TAUX-ESSAI TO WS-TAUX-HAUT
               END-IF
           END-PERFORM

      *> Le taux periodique resolu est ramene a l'annee par capitalisation.
           COMPUTE WS-TAEG ROUNDED =
               ((1 + WS-TAUX-ESSAI) ** 12 - 1) * 100.

       VALEUR-ACTUELLE-DES-ECHEANCES.
           MOVE 0 TO WS-VALEUR-ACTUELLE
           MOVE 1 TO WS-ESCOMPTE

           PERFORM VARYING WS-J FROM 1 BY 1 UNTIL WS-J > WS-E-DUREE
               COMPUTE WS-ESCOMPTE = WS-ESCOMPTE / (1 + WS-TAUX-ESSAI)
               COMPUTE WS-VALEUR-ACTUELLE = WS-VALEUR-ACTUELLE
                   + WS-PAIEMENT(WS-J) * WS-ESCOMPTE
           END-PERFORM.

       ECRIRE-RECAPITULATIF.
           MOVE WS-E-DUREE      TO WS-R-ECHEANCES
           MOVE WS-PREMIERE     TO WS-R-PREMIERE
           MOVE WS-DERNIERE     TO WS-R-DERNIERE
           MOVE WS-CUM-INTERET  TO WS-R-INTERETS
           MOVE WS-CUM-ECHEANCE TO WS-R-TOTAL
           MOVE WS-TAEG         TO WS-R-TAEG

           DISPLAY WS-RECAP.

       ECRIRE-LIGNE.
           MOVE WS-I            TO WS-L-NUMERO
           MOVE WS-ECHEANCE     TO WS-L-PAIEMENT
           MOVE WS-INTERET      TO WS-L-INTERETS
           MOVE WS-PART-CAPITAL TO WS-L-CAPITAL
           MOVE WS-SOLDE        TO WS-L-SOLDE

           DISPLAY WS-LIGNE.
