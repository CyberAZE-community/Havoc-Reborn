#include <Havoc/DBManager/DBManager.hpp>
#include <QCryptographicHash>
#include <QSqlError>

using namespace HavocNamespace;

/* teamserver passwords are stored as "sha3:<hex>" instead of plaintext;
 * legacy plaintext rows are migrated on read */
static auto HashPassword( const QString& Password ) -> QString
{
    return "sha3:" + QCryptographicHash::hash( Password.toLocal8Bit(), QCryptographicHash::Sha3_256 ).toHex();
}

bool HavocSpace::DBManager::addTeamserverInfo( const Util::ConnectionInfo& connection )
{
    auto query   = QSqlQuery();
    auto success = true;
    auto error   = std::string();

    query.prepare( "insert into Teamservers (ProfileName, Host, Port, User, Password) values(:ProfileName, :Host, :Port, :User, :Password)" );

    query.bindValue( ":ProfileName", connection.Name.toStdString().c_str() );
    query.bindValue( ":Host",        connection.Host.toStdString().c_str() );
    query.bindValue( ":Port",        connection.Port.toStdString().c_str() );
    query.bindValue( ":User",        connection.User.toStdString().c_str() );
    query.bindValue( ":Password",    connection.PasswordIsHashed ?
                                     connection.Password.toStdString().c_str() :
                                     HashPassword( connection.Password ).toStdString().c_str() );

    /* print error */
    if ( ! ( success = query.exec() ) ) {
        error   = query.lastError().text().toStdString();
        success = false;

        spdlog::error( "[DB] Failed to add teamserver info: {}", error );
    }

    return success;
}

bool HavocSpace::DBManager::checkTeamserverExists( const QString& ProfileName )
{
    auto query   = QSqlQuery();
    auto success = false;
    auto error   = std::string();

    query.prepare( "select * from Teamservers" );

    if ( ! query.exec() ) {
        error = query.lastError().text().toStdString();
        spdlog::error( "[DB] Failed to query teamserver existence: {}", error );
        return success;
    }

    while ( query.next() ) {
        if ( query.value( "ProfileName" ) == ProfileName ) {
            success = true;
            break;
        }
    }

    return success;
}

bool HavocSpace::DBManager::removeTeamserverInfo( const QString& ProfileName )
{
    auto query = QSqlQuery();
    auto error = std::string();
    auto name  = std::string();

    query.prepare( "delete from Teamservers where ProfileName = :ProfileName" );
    query.bindValue( ":ProfileName", ProfileName );

    if ( ! query.exec() ) {
        error = query.lastError().text().toStdString();
        name  = ProfileName.toStdString();

        spdlog::error( "[DB] Failed to deleting teamserver [{}] info: {}", name, error );
        return false;
    }

    return true;
}

vector<Util::ConnectionInfo> HavocSpace::DBManager::listTeamservers()
{
    auto query          = QSqlQuery();
    auto TeamserverList = vector<Util::ConnectionInfo>();
    auto error          = std::string();

    query.prepare( "select * from Teamservers" );

    if ( ! query.exec() ) {
        error = query.lastError().text().toStdString();

        spdlog::error( "[DB] Error while query teamserver list: {}", error );
        return TeamserverList;
    }

    /* iterating over the queried list */
    while ( query.next() ) {
        auto StoredPassword = query.value( "Password" ).toString();
        auto Info           = Util::ConnectionInfo {
            .Name     = query.value( "ProfileName" ).toString(),
            .Host     = query.value( "Host" ).toString(),
            .Port     = query.value( "Port" ).toString(),
            .User     = query.value( "User" ).toString(),
        };

        if ( StoredPassword.startsWith( "sha3:" ) ) {
            Info.Password         = StoredPassword.mid( 5 );
            Info.PasswordIsHashed = true;
        } else {
            /* legacy plaintext row — migrate it to the hashed form */
            Info.Password         = StoredPassword;
            Info.PasswordIsHashed = false;

            auto migrate = QSqlQuery();
            migrate.prepare( "update Teamservers set Password = :Password where ProfileName = :ProfileName" );
            migrate.bindValue( ":Password",    HashPassword( StoredPassword ) );
            migrate.bindValue( ":ProfileName", Info.Name );

            if ( ! migrate.exec() )
                spdlog::error( "[DB] Failed to migrate teamserver password hash: {}", migrate.lastError().text().toStdString() );
        }

        TeamserverList.push_back( Info );
    }

    return TeamserverList;
}

bool HavocSpace::DBManager::removeAllTeamservers()
{
    auto query = QSqlQuery();
    auto error = std::string();

    query.prepare( "delete from Teamservers" );

    if ( ! query.exec() ) {
        error = query.lastError().text().toStdString();

        spdlog::error( "[DB] Error while deleting teamservers: {}", error );

        return false;
    }

    return true;
}
