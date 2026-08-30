#include <UserInterface/Widgets/ScriptManager.h>
#include <UserInterface/Widgets/TeamserverTabSession.h>
#include <Havoc/DBManager/DBManager.hpp>

#include <UserInterface/Widgets/Store.hpp>
#include <global.hpp>

#include <QScrollBar>
#include <QProcess>

void Store::setupUi( QWidget* Store)
{
    StoreWidget = Store;
    QUrl url("https://raw.githubusercontent.com/p4p1/havoc-store/main/public/havoc-modules.json");
    QNetworkAccessManager* manager = new QNetworkAccessManager( Store );
    QNetworkReply *reply = manager->get(QNetworkRequest(url));

    if ( Store->objectName().isEmpty() )
        Store->setObjectName( QString::fromUtf8( "Extensions" ) );

    horizontalLayout = new QHBoxLayout( Store );
    horizontalLayout->setObjectName( QString::fromUtf8( "horizontalLayout" ) );

    StoreSplitter = new QSplitter();

    StoreLogger = new QTextEdit( /* PageLogger */ );
    StoreLogger->setObjectName( QString::fromUtf8( "StoreLogger" ) );
    StoreLogger->setReadOnly( true );

    panelStore = new QWidget();
    root_panelStore = new QWidget();
    panelLayout = new QVBoxLayout(root_panelStore);

    panelScroll = new QScrollArea(panelStore);
    panelScroll->setWidgetResizable(true);
    panelScroll->setWidget(root_panelStore);
    panelScroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);

    root_panelLayout = new QVBoxLayout(panelStore);
    root_panelLayout->addWidget(panelScroll);

    headerLabelTitle = new QLabel( "<h1>Havoc Extensions!</h1>", panelStore );
    headerLabelTitle->setWordWrap(true);
    panelLayout->addWidget(headerLabelTitle);
    panelLabelAuthor = new QLabel( "<span style='color:#71e0cb'>The author</span>", panelStore );
    panelLabelAuthor->setWordWrap(true);
    panelLayout->addWidget(panelLabelAuthor);
    panelLabelDescription = new QLabel( "This tab is to install extensions inside of havoc!", panelStore );
    panelLabelDescription->setWordWrap(true);
    panelLayout->addWidget(panelLabelDescription);
    installButton = new QPushButton("Install");
    installButton->setEnabled(false);
    panelLayout->addWidget(installButton);

    StoreTable = new QTableWidget();
    StoreTable->setColumnCount(2);

    labelTitle = new QTableWidgetItem( "Title"       );
    labelAuthor  = new QTableWidgetItem( "Author"      );
    StoreTable->setHorizontalHeaderItem( 0, labelTitle   );
    StoreTable->setHorizontalHeaderItem( 1, labelAuthor  );

    StoreTable->setEnabled( true );
    StoreTable->setShowGrid( false );
    StoreTable->setSortingEnabled( false );
    StoreTable->setWordWrap( true );
    StoreTable->setCornerButtonEnabled( true );
    StoreTable->horizontalHeader()->setVisible( true );
    StoreTable->setSelectionBehavior( QAbstractItemView::SelectRows );
    StoreTable->setContextMenuPolicy( Qt::CustomContextMenu );
    StoreTable->horizontalHeader()->setSectionResizeMode( QHeaderView::ResizeMode::Stretch );
    StoreTable->horizontalHeader()->setStretchLastSection( true );
    StoreTable->setEditTriggers(QAbstractItemView::NoEditTriggers);
    StoreTable->verticalHeader()->setVisible( false );
    StoreTable->setFocusPolicy( Qt::NoFocus );

    /* `this` as context object: the lambda is disconnected on teardown so a
     * late reply can't write into dangling widget state */
    QObject::connect(reply, &QNetworkReply::finished, this, [reply, this]() {
        if (reply->error() == QNetworkReply::NoError) {
            QByteArray data = reply->readAll();
            QJsonDocument jsonDoc = QJsonDocument::fromJson(data);

            if (!jsonDoc.isNull() && jsonDoc.isArray()) {
                if ( this->dataStore != nullptr )
                    delete this->dataStore;
                this->dataStore = new QJsonArray(jsonDoc.array());
                QJsonArray jsonArray = jsonDoc.array();
                int row_num = 0;
                int table_row = 0;
                for (const QJsonValue &value : jsonArray) {
                    if (value.isObject()) {
                        QJsonObject jsonObj = value.toObject();
                        QString title = jsonObj.value("title").toString();
                        QString author = jsonObj.value("author").toString();

                        QTableWidgetItem *title_widget= new QTableWidgetItem(title);
                        QTableWidgetItem *author_widget = new QTableWidgetItem(author);
                        /* the table only holds rows for object entries: the
                         * table row counter is separate from row_num (the
                         * index into dataStore) so non-object entries can't
                         * desync the mapping — row_num is stored in UserRole */
                        title_widget->setData( Qt::UserRole, row_num );
                        if ( table_row >= this->StoreTable->rowCount() )
                            this->StoreTable->insertRow( table_row );
                        this->StoreTable->setItem(table_row, 0, title_widget);
                        this->StoreTable->setItem(table_row, 1, author_widget);
                        table_row++;
                    }
                    row_num++;
                }
            } else {
                spdlog::error( "[STORE] Failed to parse the JSON data" );
            }
        } else {
            spdlog::error( "[STORE] Failed to get json from web" );
        }

        reply->deleteLater();
    });

    QObject::connect(StoreTable, &QTableWidget::itemSelectionChanged, [this]() {
        QList<QTableWidgetItem *> selectedItems = this->StoreTable->selectedItems();
        if (!selectedItems.isEmpty()) {
            auto* titleItem = this->StoreTable->item( selectedItems.first()->row(), 0 );
            if ( titleItem != nullptr )
                displayData( titleItem->data( Qt::UserRole ).toInt() );
        }
    });

    QObject::connect(installButton, &QPushButton::clicked, [this]() {
        QList<QTableWidgetItem *> selectedItems = this->StoreTable->selectedItems();
        if (!selectedItems.isEmpty()) {
            auto* titleItem = this->StoreTable->item( selectedItems.first()->row(), 0 );
            if ( titleItem != nullptr )
                installScript( titleItem->data( Qt::UserRole ).toInt() );
        }
    });


    StoreSplitter->addWidget( StoreTable );
    StoreSplitter->addWidget( panelStore );

    horizontalLayout->addWidget( StoreSplitter );

    retranslateUi(  );

    QMetaObject::connectSlotsByName( StoreWidget );
}

void Store::displayData(int position)
{
    if ( dataStore == nullptr || position < 0 || position >= dataStore->size() )
        return;

    QJsonObject jsonObj = dataStore->at(position).toObject();
    QString title = jsonObj.value("title").toString();
    QString description = jsonObj.value("description").toString();
    QString author = jsonObj.value("author").toString();

    headerLabelTitle->setText(QString("<h1>%1</h1>").arg(title.toHtmlEscaped()));
    panelLabelDescription->setText(description.toHtmlEscaped());
    panelLabelAuthor->setText(QString("<span style='color:#71e0cb'>%1</span>").arg(author.toHtmlEscaped()));
    installButton->setEnabled(true);
}

bool Store::AddScript( QString Path )
{
    auto Script = FileRead( Path );
    auto path   = Path.toStdString();
    int  Return = 0;

    HavocX::Teamserver.LoadingScript = Path.toStdString();

    if ( Script != nullptr ) {
        if ( ! Script.isEmpty() ) {
            Return = PyRun_SimpleStringFlags( Script.toStdString().c_str(), NULL );
            if ( Return == -1 ) {
                spdlog::error( "Failed to run script: {}", path );
            } else {
                return true;
            }
        }
        else {
            spdlog::error( "Script path not found: {}", path );
        }
    } else {
        spdlog::error( "Failed to load script: {}", path );
    }

    HavocX::Teamserver.LoadingScript = "";

    return false;
}

void Store::installScript(int position)
{
    if ( dataStore == nullptr || position < 0 || position >= dataStore->size() )
        return;

    QString gistUrl = "https://gist.githubusercontent.com/%1/%2/raw/%3";

    QJsonObject jsonObj = dataStore->at(position).toObject();
    QString url = jsonObj.value("link").toString();
    QString entrypoint = jsonObj.value("entrypoint").toString();
    QString author = jsonObj.value("author").toString();

    /* the store index is untrusted network data: names that carry path
     * components would make wget/git write (and AddScript then execute)
     * files outside data/extensions */
    auto isSafeName = []( const QString& name ) {
        return ! name.isEmpty() && ! name.contains( '/' ) && ! name.contains( '\\' ) && ! name.contains( ".." );
    };
    if ( ! isSafeName( entrypoint ) ) {
        spdlog::error( "[STORE] rejecting unsafe entrypoint in store index" );
        return;
    }

    QString currentPath = QDir::currentPath();
    QDir extension_path(QString("./data/extensions"));

    if (!extension_path.exists()) {
        QDir tmp_dir(currentPath);
        tmp_dir.mkpath(QString("./data/extensions"));
    }
    int is_gist = url.indexOf(QString("gist.github.com"));
    if (is_gist != -1) {
        QStringList urlParts = url.split('/');
        QString github_hash = urlParts.last();

        QString downloadURL = gistUrl.arg(author, github_hash, entrypoint);
        QString pathScript = QString("%1/data/extensions/%2").arg(currentPath).arg(entrypoint);

        // store index data is untrusted: never pass it through a shell
        if ( QProcess::execute( QString("wget"), QStringList{ downloadURL, "-O", pathScript } ) != 0 ) {
            spdlog::error( "[STORE] wget failed to download {}", downloadURL.toStdString() );
            return;
        }

        if ( AddScript( pathScript ) ) {
            if ( ! HavocX::Teamserver.TabSession->dbManager->CheckScript(pathScript) )
                HavocX::Teamserver.TabSession->dbManager->AddScript(pathScript);
        }
    } else { // Must be a repo then and not a gist ^^ now we can be happy for that entrypoint var
        QStringList urlParts = url.split('/');
        QString repo_name = urlParts.last();

        if ( ! isSafeName( repo_name ) ) {
            spdlog::error( "[STORE] rejecting unsafe repository name in store index" );
            return;
        }

        QString pathScript = QString("%1/data/extensions/%2/%3").arg(currentPath).arg(repo_name).arg(entrypoint);
        QString cloneTarget = QString("%1/data/extensions/%2").arg(currentPath).arg(repo_name);

        // store index data is untrusted: never pass it through a shell
        if ( QProcess::execute( QString("git"), QStringList{ "clone", "--recurse-submodules", "--remote-submodules", url, cloneTarget } ) != 0 ) {
            spdlog::error( "[STORE] git clone failed for {}", url.toStdString() );
            return;
        }

        if ( AddScript( pathScript ) ) {
            if ( ! HavocX::Teamserver.TabSession->dbManager->CheckScript(pathScript) )
                HavocX::Teamserver.TabSession->dbManager->AddScript(pathScript);
        }
    }
}

void Store::retranslateUi()
{
    StoreWidget->setWindowTitle( QCoreApplication::translate( "Extensions", "Extensions", nullptr ) );
}
